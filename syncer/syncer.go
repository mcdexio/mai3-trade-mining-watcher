package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
)

var NOT_IN_EPOCH = errors.New("not in epoch period")
var EMPTY_SCHEDULE = errors.New("empty schedule")
var minuteDecimal = decimal.NewFromInt(60)

var (
	PROGRESS_SYNC_STATE = "user_info" // compatible
	PROGRESS_INIT_FEE   = "user_info.init_fee"
)

var INVERSE_CONTRACT_WHITELIST = map[string]bool{
	"0x3d3744dc7a17d757a2568ddb171d162a7e12f80-0":  true, // 10689
	"0x727e5a9a04080741cbc8a2dc891e28ca8af6537e-0": true, // 10690
}

type Syncer struct {
	ctx        context.Context
	httpClient *utils.Client
	logger     logging.Logger
	db         *gorm.DB

	// block syncer
	blockGraphInterface graph.BlockInterface
	mai3GraphInterface  graph.MAI3Interface

	// weight
	curEpochConfig *mining.Schedule

	// default if you don't set epoch in schedule database
	defaultEpochStartTime int64
}

func NewSyncer(
	ctx context.Context, logger logging.Logger, mai3GraphUrl string, blockGraphUrl string,
	defaultEpochStartTime int64) *Syncer {
	return &Syncer{
		ctx:                   ctx,
		logger:                logger,
		mai3GraphInterface:    graph.NewMAI3Client(logger, mai3GraphUrl),
		blockGraphInterface:   graph.NewBlockClient(logger, blockGraphUrl),
		db:                    database.GetDB(),
		defaultEpochStartTime: defaultEpochStartTime,
	}
}

func (s *Syncer) setDefaultEpoch() int64 {
	// start := time.Now().Unix()
	start := s.defaultEpochStartTime
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "epoch"}},
		UpdateAll: true,
	}).Create(&mining.Schedule{
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
	for {
		if err := s.run(s.ctx); err != nil {
			if !errors.Is(err, NOT_IN_EPOCH) {
				s.logger.Warn("error occurs while running: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		s.logger.Warn("not in any epoch")
		time.Sleep(5 * time.Second)
	}
}

func (s *Syncer) run(ctx context.Context) error {
	// try to find out last epoch
	cp, err := s.lastProgress(PROGRESS_SYNC_STATE)
	if err != nil {
		return err
	}
	// brand new start, no last progress
	if cp == 0 {
		// on very init state, here the cp should be 0
		// which means it is impossible to detect which epoch we are in
		// so set default epoch information from bin/config DEFAULT_EPOCH_0_START_TIME
		cp = s.setDefaultEpoch()
	}
	e, err := s.detectEpoch(cp)
	if err != nil {
		return err
	}
	s.logger.Info("found in epoch %+v", e)
	// set epoch
	s.curEpochConfig = e
	// set init fee
	if err := s.initUserStates(); err != nil {
		return fmt.Errorf("fail to init user states: %w", err)
	}
	// sync state
	np := cp
	for np < s.curEpochConfig.EndTime {
		select {
		case <-ctx.Done():
			return nil
		default:
			p, err := s.syncState()
			if err != nil {
				s.logger.Warn("fail to sync state, retry in 5 seconds %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
			np = p + 60
			now := norm(time.Now().Unix())
			if np > now && np < s.curEpochConfig.EndTime {
				// sleep until next tick
				time.Sleep(time.Duration(np-now) * time.Second)
			}
		}
	}
	s.logger.Info("epoch done: epoch=%+v", s.curEpochConfig)
	return nil
}

func (s *Syncer) GetPoolAddrIndexUserID(marginAccountID string) (poolAddr, userId string, perpetualIndex int, err error) {
	rest := strings.Split(marginAccountID, "-")
	perpetualIndex, err = strconv.Atoi(rest[1])
	if err != nil {
		return
	}
	poolAddr = rest[0]
	userId = rest[2]
	return
}

func (s *Syncer) lastProgress(name string) (int64, error) {
	var p mining.Progress
	err := s.db.Model(mining.Progress{}).Where("table_name=?", name).Order("epoch desc").First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get last progress: table=%s %w", name, err)
	}
	return p.From, nil
}

func (s *Syncer) getProgress(name string, epoch int64) (int64, error) {
	var p mining.Progress
	err := s.db.Model(mining.Progress{}).Where("table_name=? and epoch=?", name, epoch).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get progress: table=%s %w", name, err)
	}
	return p.From, nil
}

func (s *Syncer) setProgress(name string, ts int64, epoch int64) error {
	s.logger.Info("save progress for %v: timestamp=%v", name, ts)
	p := &mining.Progress{TableName: types.TableName(name), From: ts, Epoch: epoch}
	if err := s.db.Save(p).Error; err != nil {
		return fmt.Errorf("fail to save progress: table=%v, timestamp=%v %w", name, ts, err)
	}
	return nil
}

func (s *Syncer) initUserStates() error {
	s.logger.Debug("enter initUserStates")
	defer s.logger.Debug("leave initUserStates")
	p, err := s.getProgress(PROGRESS_INIT_FEE, s.curEpochConfig.Epoch)
	if err != nil {
		return fmt.Errorf("fail to get sync progress %w", err)
	}
	// already synced
	if p != 0 && p == s.curEpochConfig.StartTime {
		s.logger.Info("fee already initialied")
		return nil
	}
	// query all total fee before this epoch start time, if not exist, return
	startBn, err := s.getTimestampToBlockNumber(s.curEpochConfig.StartTime, s.blockGraphInterface)
	if err != nil {
		return err
	}
	users, err := s.getUsersBasedOnBlockNumber(startBn, s.mai3GraphInterface)
	if err != nil {
		return err
	}
	uis := make([]*mining.UserInfo, len(users))
	for i, u := range users {
		uis[i] = &mining.UserInfo{
			Trader:  strings.ToLower(u.ID),
			Epoch:   s.curEpochConfig.Epoch,
			InitFee: u.TotalFee,
		}
	}
	err = db.WithTransaction(s.db, func(tx *gorm.DB) error {
		cp, err := s.getProgress(PROGRESS_INIT_FEE, s.curEpochConfig.Epoch)
		if err != nil {
			return fmt.Errorf("fail to get sync progress %w", err)
		}
		// safe guard
		if cp != p {
			return fmt.Errorf("progress changed, somewhere may run another instance")
		}
		if err := s.db.Model(mining.UserInfo{}).Create(&uis).Error; err != nil {
			return fmt.Errorf("fail to create init user info %w", err)
		}
		if err := s.setProgress(PROGRESS_INIT_FEE, s.curEpochConfig.StartTime, s.curEpochConfig.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("fail to init fee of all users for new epoch: epoch=%v %w", s.curEpochConfig.Epoch, err)
	}
	return nil
}

func (s *Syncer) syncState() (int64, error) {
	s.logger.Info("enter sync state")
	defer s.logger.Info("leave sync state")
	p, err := s.getProgress(PROGRESS_SYNC_STATE, s.curEpochConfig.Epoch)
	if err != nil {
		return 0, fmt.Errorf("fail to get sync progress %w", err)
	}
	var (
		lp int64
		np int64
	)
	if p == 0 {
		lp = s.curEpochConfig.StartTime
	} else {
		lp = p
	}
	np = norm(lp + 60)
	bn, err := s.getTimestampToBlockNumber(np, s.blockGraphInterface)
	if err != nil {
		return 0, fmt.Errorf("failed to get block number from timestamp: timestamp=%v %w", np, err)
	}
	users, err := s.getUsersBasedOnBlockNumber(bn, s.mai3GraphInterface)
	if err != nil {
		return 0, fmt.Errorf("failed to get users on block number: blocknumber=%v %w", bn, err)
	}
	s.logger.Info("found %v users @%v", len(users), bn)
	// 2. update graph data
	prices, err := s.getMarkPrice(bn, s.mai3GraphInterface)
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
		ss := s.getStakeScore(np, u.UnlockMCBTime, u.StakedMCB)
		ui := &mining.UserInfo{
			Trader:        strings.ToLower(u.ID),
			Epoch:         s.curEpochConfig.Epoch,
			CurPosValue:   pv,
			CurStakeScore: ss,
			AccFee:        u.TotalFee,
		}
		uis[i] = ui
	}
	// begin tx
	err = db.WithTransaction(s.db, func(tx *gorm.DB) error {
		curP, err := s.getProgress(PROGRESS_SYNC_STATE, s.curEpochConfig.Epoch)
		if err != nil {
			return fmt.Errorf("fail to get sync progress %w", err)
		}
		if curP != p {
			return fmt.Errorf("progress changed, somewhere may run another instance")
		}
		// acc_pos_value += cur_pos_value if cur_pos_value != 0
		err = s.db.Model(mining.UserInfo{}).
			Where("epoch=? and cur_pos_value <> 0", s.curEpochConfig.Epoch).
			UpdateColumn("acc_pos_value", gorm.Expr("acc_pos_value + cur_pos_value")).Error
		if err != nil {
			return fmt.Errorf("failed to accumulate cur_post_value to acc_pos_value  %w", err)
		}
		// acc_stake_score += cur_stake_score if cur_stake_score != 0
		err = s.db.Model(mining.UserInfo{}).
			Where("epoch=? and cur_stake_score <> 0", s.curEpochConfig.Epoch).
			UpdateColumn("acc_stake_score", gorm.Expr("acc_stake_score + cur_stake_score")).Error
		if err != nil {
			return fmt.Errorf("failed to accumulate cur_stake_score to acc_stake_score %w", err)
		}
		// cur_stake_score <= 0 and cur_pos_value <= 0
		err = s.db.Model(mining.UserInfo{}).Where("epoch=?", s.curEpochConfig.Epoch).
			Updates(mining.UserInfo{CurPosValue: decimal.Zero, CurStakeScore: decimal.Zero, Timestamp: np}).Error
		if err != nil {
			return fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
			DoUpdates: clause.AssignmentColumns([]string{"cur_pos_value", "cur_stake_score", "acc_fee"}),
		}).Create(&uis).Error; err != nil {
			return fmt.Errorf("failed to create user_info: size=%v %w", len(uis), err)
		}
		// 3. update score
		var (
			minuteCeil = int64(math.Ceil((float64(np) - float64(s.curEpochConfig.StartTime)) / 60.0))
			elapsed    = decimal.NewFromInt(minuteCeil) // Minutes
		)
		var all []*mining.UserInfo
		if err := s.db.Model(mining.UserInfo{}).Where("epoch=?", s.curEpochConfig.Epoch).Find(&all).Error; err != nil {
			return fmt.Errorf("fail to fetch all users in this epoch %w", err)
		}
		for _, ui := range all {
			ui.Score = s.getScore(ui, elapsed)
		}
		if err := s.db.Save(&all).Error; err != nil {
			return fmt.Errorf("failed to create user_info: size=%v %w", len(uis), err)
		}
		if err := s.setProgress(PROGRESS_SYNC_STATE, np, s.curEpochConfig.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("fail to update state to db %w", err)
	}
	return np, nil
}

func (s Syncer) getStakeScore(curTime int64, unlockTime int64, staked decimal.Decimal) decimal.Decimal {
	if unlockTime < curTime {
		return decimal.Zero
	}
	// floor to 1 if less than 1 day
	days := int64(math.Ceil(float64(unlockTime-curTime) / 86400))
	return decimal.NewFromInt(days).Mul(staked)
}

func (s Syncer) getScore(ui *mining.UserInfo, elapsed decimal.Decimal) decimal.Decimal {
	if ui.AccFee.IsZero() {
		return decimal.Zero
	}
	fee := ui.AccFee.Sub(ui.InitFee)
	if fee.IsZero() {
		return decimal.Zero
	}
	stake := ui.AccStakeScore.Add(ui.CurStakeScore)
	if stake.IsZero() {
		return decimal.Zero
	}
	posVal := ui.AccPosValue.Add(ui.CurPosValue)
	if posVal.IsZero() {
		return decimal.Zero
	}

	// decimal package has issue on pow function
	elapsedFloat, _ := elapsed.Float64()
	wFee, _ := s.curEpochConfig.WeightFee.Float64()
	wStake, _ := s.curEpochConfig.WeightMCB.Float64()
	wPos, _ := s.curEpochConfig.WeightOI.Float64()
	feeFloat, _ := fee.Float64()
	stakeFloat, _ := stake.Float64()
	posValFloat, _ := posVal.Float64()
	score := math.Pow(feeFloat, wFee) * math.Pow(stakeFloat/elapsedFloat, wStake) * math.Pow(posValFloat/elapsedFloat, wPos)
	return decimal.NewFromFloat(score)
}

func (s Syncer) getPositionValue(accounts []*graph.MarginAccount, bn int64, cache map[string]decimal.Decimal) (decimal.Decimal, error) {
	sum := decimal.Zero
	for _, a := range accounts {
		var price decimal.Decimal

		// 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0-0x00233150044aec4cba478d0bf0ecda0baaf5ad19
		perpId := strings.Join(strings.Split(a.ID, "-")[:2], "-") // 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0

		// inverse contract
		if env.InInverseContractWhiteList(perpId) {
			sum = sum.Add(a.Position.Abs())
			continue
		}

		// normal contract
		if v, ok := cache[perpId]; ok {
			price = v
		} else {
			addr, _, index, err := s.GetPoolAddrIndexUserID(a.ID)
			if err != nil {
				return sum, fmt.Errorf("fail to get pool address and index from id %w", err)
			}
			p, err := s.getMarkPriceWithBlockNumberAddrIndex(bn, addr, index, s.mai3GraphInterface)
			if err != nil {
				return sum, fmt.Errorf("fail to get mark price %w", err)
			}
			price = p
			cache[perpId] = p
		}
		sum = sum.Add(price.Mul(a.Position).Abs())
	}
	return sum, nil
}

func (s *Syncer) detectEpoch(p int64) (*mining.Schedule, error) {
	// get epoch from schedule database.
	// detect which epoch is time(p) in.
	var ss []*mining.Schedule
	// handle overlap in setEpoch of internalServer
	if err := s.db.Model(&mining.Schedule{}).Where("end_time>?", p+60).Order("epoch asc").Find(&ss).Error; err != nil {
		return nil, fmt.Errorf("fail to found epoch config %w", err)
	}
	if len(ss) == 0 {
		// not in any epoch
		return nil, NOT_IN_EPOCH
	}
	return ss[0], nil
}

func (s *Syncer) getUsersBasedOnBlockNumber(blockNumber int64, mai3Interface graph.MAI3Interface) ([]graph.User, error) {
	return mai3Interface.GetUsersBasedOnBlockNumber(blockNumber)
}

func (s *Syncer) getMarkPrice(blockNumber int64, mai3Interface graph.MAI3Interface) (map[string]decimal.Decimal, error) {
	return mai3Interface.GetMarkPrices(blockNumber)
}

func (s *Syncer) getMarkPriceWithBlockNumberAddrIndex(
	blockNumber int64, poolAddr string, perpetualIndex int, mai3Interface graph.MAI3Interface) (decimal.Decimal, error) {
	return mai3Interface.GetMarkPriceWithBlockNumberAddrIndex(blockNumber, poolAddr, perpetualIndex)
}

func (s *Syncer) getTimestampToBlockNumber(timestamp int64, blockInterface graph.BlockInterface) (int64, error) {
	return blockInterface.GetTimestampToBlockNumber(timestamp)
}
