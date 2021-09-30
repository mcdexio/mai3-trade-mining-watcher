package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
)

type Syncer struct {
	ctx    context.Context
	logger logging.Logger
	db     *gorm.DB
	dao    database.DAO

	// block syncer
	graphCli     *GraphClient
	calc         ScoreCalculator
	defaultEpoch int64 // // default if you don't set epoch in schedule database
}

func NewSyncer(
	ctx context.Context,
	logger logging.Logger,
	mai3GraphUrl string,
	blockGraphUrl string,
	defaultEpoch int64,
) *Syncer {
	return &Syncer{
		ctx:          ctx,
		logger:       logger,
		db:           database.GetDB(),
		defaultEpoch: defaultEpoch,
		graphCli: &GraphClient{
			MaxRetry:      3,
			RetryDelay:    3 * time.Second,
			Mai3GraphUrl:  mai3GraphUrl,
			BlockGraphUrl: blockGraphUrl,
			hc: utils.NewHttpClient(&http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 500 * time.Millisecond,
				}).DialContext,
				TLSHandshakeTimeout: 1000 * time.Millisecond,
				MaxIdleConns:        100,
				IdleConnTimeout:     30 * time.Second,
			}, logger),
		},
	}
}

func (s *Syncer) setDefaultEpoch() int64 {
	var start int64 = 1632972600
	err := s.db.Create(&mining.Schedule{
		Epoch:     0,
		StartTime: start,
		EndTime:   start + 60*60*24*14,
		WeightFee: decimal.NewFromFloat(0.7),
		WeightMCB: decimal.NewFromFloat(0.3),
		WeightOI:  decimal.NewFromFloat(0.3),
	}).Error
	if err != nil {
		s.logger.Error("set default epoch error %s", err)
		panic(err)
	}
	return start
}

func (s *Syncer) Run() error {
	// TODO(yz): for test purpose, remove before mainnet deployment
	s.setDefaultEpoch()
	for {
		if err := s.run(s.ctx); err != nil {
			if !errors.Is(err, database.ErrNotInEpoch) {
				s.logger.Warn("error occurs while running: %w", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		s.logger.Warn("not in any epoch")
		time.Sleep(5 * time.Second)
	}
}

func (s *Syncer) run(ctx context.Context) error {
	e, err := s.dao.DetectEpoch(s.db, s.defaultEpoch)
	if err != nil {
		return err
	}
	s.logger.Info("found in epoch %+v", e)
	// set init fee
	if err := s.initUserStates(e); err != nil {
		return fmt.Errorf("fail to init user states: %w", err)
	}
	// sync state
	var np int64
	for np < e.EndTime {
		select {
		case <-ctx.Done():
			return nil
		default:
			p, err := s.syncState(e)
			if err != nil {
				s.logger.Warn("fail to sync state, retry in 5 seconds %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
			np = p + 60
			now := norm(time.Now().Unix())
			if np > now && np < e.EndTime {
				// sleep until next tick
				time.Sleep(time.Duration(np-now) * time.Second)
			}
		}
	}
	s.logger.Info("epoch done: epoch=%+v", e)
	return nil
}

func (s *Syncer) initUserStates(epoch *mining.Schedule) error {
	s.logger.Debug("enter initUserStates")
	defer s.logger.Debug("leave initUserStates")

	p, err := s.dao.GetInitProgress(s.db, epoch.Epoch)
	if err != nil {
		return fmt.Errorf("fail to get sync progress %w", err)
	}
	// already synced
	if p != 0 && p == epoch.StartTime {
		s.logger.Info("fee already initialied")
		return nil
	}
	// query all total fee before this epoch start time, if not exist, return
	startBn, err := s.graphCli.TimestampToBlockNumber(epoch.StartTime)
	if err != nil {
		return err
	}
	users, err := s.graphCli.GetUsersBasedOnBlockNumber(startBn)
	if err != nil {
		return err
	}
	uis := make([]*mining.UserInfo, len(users))
	for i, u := range users {
		uis[i] = &mining.UserInfo{
			Trader:  strings.ToLower(u.ID),
			Epoch:   epoch.Epoch,
			InitFee: u.TotalFee,
		}
	}
	err = database.WithTransaction(s.db, func(tx *gorm.DB) error {
		cp, err := s.dao.GetInitProgress(tx, epoch.Epoch)
		if err != nil {
			return fmt.Errorf("fail to get sync progress %w", err)
		}
		// safe guard
		if cp != p {
			return fmt.Errorf("progress changed, somewhere may run another instance")
		}
		if err := tx.Model(mining.UserInfo{}).Create(&uis).Error; err != nil {
			return fmt.Errorf("fail to create init user info %w", err)
		}
		if err := s.dao.SaveInitProgress(tx, epoch.StartTime, epoch.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("fail to init fee of all users for new epoch: epoch=%v %w", epoch.Epoch, err)
	}
	return nil
}

func (s *Syncer) syncState(epoch *mining.Schedule) (int64, error) {
	s.logger.Info("enter sync state")
	defer s.logger.Info("leave sync state")
	start := time.Now()

	p, err := s.dao.GetSyncProgress(s.db, epoch.Epoch)
	if err != nil {
		return 0, fmt.Errorf("fail to get sync progress %w", err)
	}
	var (
		lp int64
		np int64
	)
	if p == 0 {
		lp = epoch.StartTime
	} else {
		lp = p
	}
	np = norm(lp + 60)
	bn, err := s.graphCli.TimestampToBlockNumber(np)
	if err != nil {
		return 0, fmt.Errorf("failed to get block number from timestamp: timestamp=%v %w", np, err)
	}
	users, err := s.graphCli.GetUsersBasedOnBlockNumber(bn)
	if err != nil {
		return 0, fmt.Errorf("failed to get users on block number: blocknumber=%v %w", bn, err)
	}
	s.logger.Info("found %v users @%v", len(users), bn)
	// 2. update graph data
	prices, err := s.graphCli.GetAllMarkPricesByBlock(bn)
	if err != nil {
		return 0, fmt.Errorf("fail to get mark prices %w", err)
	}
	uis := make([]*mining.UserInfo, len(users))
	for i, u := range users {
		pv, err := s.getPositionValue(u.MarginAccounts, bn, prices)
		if err != nil {
			return 0, fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
		}
		// ss is (unlock time - now) * u.StackedMCB <=> s = n * t
		ss := s.calc.calcStakeScore(u, np)
		ui := &mining.UserInfo{
			Trader:        strings.ToLower(u.ID),
			Epoch:         epoch.Epoch,
			CurPosValue:   pv,
			CurStakeScore: ss,
			AccFee:        u.TotalFee,
		}
		uis[i] = ui
	}
	// begin tx
	err = database.WithTransaction(s.db, func(tx *gorm.DB) error {
		curP, err := s.dao.GetSyncProgress(tx, epoch.Epoch)
		if err != nil {
			return fmt.Errorf("fail to get sync progress %w", err)
		}
		if curP != p {
			return fmt.Errorf("progress changed, somewhere may run another instance")
		}
		// acc_pos_value += cur_pos_value if cur_pos_value != 0
		// acc_stake_score += cur_stake_score if cur_stake_score != 0
		if err := s.dao.AccumulateCurValues(tx, epoch.Epoch); err != nil {
			return err
		}
		// cur_stake_score <= 0 and cur_pos_value <= 0
		if err := s.dao.ResetCurValues(tx, epoch.Epoch, np); err != nil {
			return err
		}
		// update cur state
		if err := s.dao.UpdateUserInfo(tx, uis); err != nil {
			return err
		}
		// 3. update score
		elapsed := int64(math.Ceil((float64(p) - float64(epoch.StartTime)) / 60.0))
		all, err := s.dao.GetAllUserInfo(tx, epoch.Epoch)
		if err != nil {
			return fmt.Errorf("fail to fetch all users in this epoch %w", err)
		}
		for _, ui := range all {
			ui.Score = s.calc.calcScore(epoch, ui, elapsed)
		}
		if err := tx.Save(&all).Error; err != nil {
			return fmt.Errorf("failed to create user_info: size=%v %w", len(uis), err)
		}
		if err := s.dao.SaveSyncProgress(tx, np, epoch.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("fail to update state to db %w", err)
	}

	s.logger.Info("sync state completed: progress=%v(%v), spent=%v epoch=%+v",
		np, time.Unix(np, 0).UTC().Format("2006-01-02 15:04:05"), time.Since(start).Milliseconds(), epoch)
	return np, nil
}

func (s Syncer) getPositionValue(accounts []*MarginAccount, bn int64, cache map[string]*decimal.Decimal) (decimal.Decimal, error) {
	sum := decimal.Zero
	for _, a := range accounts {
		var price *decimal.Decimal
		// 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0-0x00233150044aec4cba478d0bf0ecda0baaf5ad19
		// => 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0
		perpId := strings.Join(strings.Split(a.ID, "-")[:2], "-")
		if v, ok := cache[perpId]; ok {
			price = v
		} else {
			return sum, fmt.Errorf("unexpected mark price: id=%v", perpId)
		}
		sum = price.Mul(a.Position).Abs()
	}
	return sum, nil
}
