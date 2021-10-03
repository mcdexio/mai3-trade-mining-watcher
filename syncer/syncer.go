package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/atomic"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
)

var ErrNotInEpoch = errors.New("not in epoch period")

var (
	PROGRESS_SYNC_STATE = "user_info" // compatible
	PROGRESS_INIT_FEE   = "user_info.init_fee"
	PROGRESS_SNAPSHOT   = "user_info.snapshot"
)

type Syncer struct {
	ctx    context.Context
	logger logging.Logger
	db     *gorm.DB

	// block syncer
	blockGraphInterface graph.BlockInterface
	mai3GraphInterface  graph.MAI3Interface

	// default if you don't set epoch in schedule database
	defaultEpochStartTime int64
	syncDelaySeconds      int64

	needRestore      atomic.Bool
	restoreTimestamp int64
	snapshotInterval int64
}

func NewSyncer(
	ctx context.Context, logger logging.Logger, mai3GraphUrl string, blockGraphUrl string,
	defaultEpochStartTime int64, syncDelaySeconds int64) *Syncer {
	return &Syncer{
		ctx:                   ctx,
		logger:                logger,
		mai3GraphInterface:    graph.NewMAI3Client(logger, mai3GraphUrl),
		blockGraphInterface:   graph.NewBlockClient(logger, blockGraphUrl),
		db:                    database.GetDB(),
		defaultEpochStartTime: defaultEpochStartTime,
		syncDelaySeconds:      syncDelaySeconds,
		snapshotInterval:      3600,
	}
}

func (s *Syncer) SetDefaultEpoch() int64 {
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
	// s.SetDefaultEpoch()
	for {
		var err error
		switch s.needRestore.Load() {
		case true:
			err = s.runRestore(s.ctx, s.restoreTimestamp)
			if err == nil {
				s.needRestore.Store(true)
			}
		default:
			err = s.runSync(s.ctx, s.db)
		}
		if err != nil {
			if !errors.Is(err, ErrNotInEpoch) {
				s.logger.Warn("error occurs while running: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		s.logger.Warn("not in any epoch")
		time.Sleep(5 * time.Second)
	}
}

func (s *Syncer) runRestore(ctx context.Context, checkpoint int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// restore from snapshot
		if err := s.restoreFromSnapshot(tx, checkpoint); err != nil {
			return err
		}
		// sync until end
		return s.runSync(ctx, tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
}

func (s *Syncer) restoreFromSnapshot(db *gorm.DB, checkpoint int64) error {
	epoch, err := s.detectEpoch(db, checkpoint)
	if err != nil {
		return err
	}
	// copy from snapshot
	var snapshots []*mining.Snapshot
	if err := db.Where("epoch=? and timestamp=?", epoch.Epoch, checkpoint).Find(&snapshots).Error; err != nil {
		return err
	}
	users := make([]*mining.UserInfo, len(snapshots))
	for i, s := range snapshots {
		users[i] = &mining.UserInfo{
			Trader:        s.Trader,
			Epoch:         s.Epoch,
			InitFee:       s.InitFee,
			AccFee:        s.AccFee,
			AccPosValue:   s.AccPosValue,
			CurPosValue:   s.CurPosValue,
			AccStakeScore: s.AccStakeScore,
			CurStakeScore: s.CurStakeScore,
			Score:         s.Score,
			Timestamp:     s.Timestamp,
		}
	}
	if err := db.Save(users).Error; err != nil {
		return err
	}
	if err := s.setProgress(db, PROGRESS_INIT_FEE, epoch.StartTime, epoch.Epoch); err != nil {
		return err
	}
	if err := s.setProgress(db, PROGRESS_SYNC_STATE, checkpoint, epoch.Epoch); err != nil {
		return err
	}
	return nil
}

// sync until now or current epoch end
func (s *Syncer) runSync(ctx context.Context, db *gorm.DB) error {
	lastTs, err := s.getLastProgress(db, PROGRESS_SYNC_STATE)
	if err != nil {
		return err
	}
	epoch, err := s.detectEpoch(db, lastTs)
	if err != nil {
		return err
	}
	s.logger.Info("found current or next epoch %+v", epoch)
	// set epoch
	curTs := lastTs + 60
	// cur not arrived
	if curTs > s.nowWithDelay() {
		s.logger.Info("sync delayed or not started")
		return nil
	}
	// set init fee
	if err := s.initUserStates(db, epoch); err != nil {
		return fmt.Errorf("fail to init user states: %w", err)
	}
	for curTs < epoch.EndTime {
		select {
		case <-ctx.Done():
			return nil
		default:
			doneTs, err := s.syncState(db, epoch)
			if err != nil {
				s.logger.Warn("fail to sync state, retry in 5 seconds %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
			curTs = doneTs + 60
			now := s.nowWithDelay()
			if curTs > now && curTs < epoch.EndTime {
				// sleep until next tick
				time.Sleep(time.Duration(curTs-now) * time.Second)
			}
		}
	}
	s.logger.Info("epoch done: epoch=%+v", epoch)
	return nil
}

func (s *Syncer) nowWithDelay() int64 {
	now := time.Now().Unix()
	if s.syncDelaySeconds != 0 {
		now = now - s.syncDelaySeconds
	}
	return now
}

func (s *Syncer) getLastProgress(db *gorm.DB, name string) (int64, error) {
	var p mining.Progress
	if err := db.Model(mining.Progress{}).Where("table_name=?", name).Order("epoch desc").First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get last progress: table=%s %w", name, err)
	}
	return p.From, nil
}

func (s *Syncer) getProgress(db *gorm.DB, name string, epoch int64) (int64, error) {
	var p mining.Progress
	err := db.Where("table_name=? and epoch=?", name, epoch).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get progress: table=%s %w", name, err)
	}
	return p.From, nil
}

func (s *Syncer) setProgress(db *gorm.DB, name string, ts int64, epoch int64) error {
	s.logger.Info("save progress for %v: timestamp=%v", name, ts)
	p := &mining.Progress{TableName: types.TableName(name), From: ts, Epoch: epoch}
	if err := db.Save(p).Error; err != nil {
		return fmt.Errorf("fail to save progress: table=%v, timestamp=%v %w", name, ts, err)
	}
	return nil
}

func (s *Syncer) initUserStates(db *gorm.DB, epoch *mining.Schedule) error {
	s.logger.Debug("enter initUserStates")
	defer s.logger.Debug("leave initUserStates")

	p, err := s.getProgress(db, PROGRESS_INIT_FEE, epoch.Epoch)
	if err != nil {
		return fmt.Errorf("fail to get sync progress %w", err)
	}
	// already synced
	if p != 0 && p == epoch.StartTime {
		s.logger.Info("fee already initialized")
		return nil
	}
	startBn, err := s.getTimestampToBlockNumber(epoch.StartTime, s.blockGraphInterface)
	if err != nil {
		return err
	}
	users, err := s.getUsersBasedOnBlockNumber(startBn, s.mai3GraphInterface)
	if err != nil {
		return err
	}
	prices, err := s.getMarkPrice(startBn, s.mai3GraphInterface)
	if err != nil {
		return fmt.Errorf("fail to get mark prices %s", err)
	}
	uis := make([]*mining.UserInfo, len(users))
	for i, u := range users {
		_, initFee, err := s.getOIFeeValue(u.MarginAccounts, startBn, prices)
		if err != nil {
			return fmt.Errorf("fail to get initial fee %s", err)
		}
		uis[i] = &mining.UserInfo{
			Trader:  strings.ToLower(u.ID),
			Epoch:   epoch.Epoch,
			InitFee: initFee,
		}
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if len(uis) > 0 {
			if err := db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
				DoUpdates: clause.AssignmentColumns([]string{"epoch", "init_fee"}),
			}).Create(&uis).Error; err != nil {
				return fmt.Errorf("fail to create init user info %w", err)
			}
		}
		if err := s.setProgress(db, PROGRESS_INIT_FEE, epoch.StartTime, epoch.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("fail to init fee of all users for new epoch: epoch=%v %w", epoch.Epoch, err)
	}
	return nil
}

func (s *Syncer) getUserStateBasedOnBlockNumber(epoch *mining.Schedule, timestamp int64) ([]*mining.UserInfo, error) {
	bn, err := s.getTimestampToBlockNumber(timestamp, s.blockGraphInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to get block number from timestamp: timestamp=%v %w", timestamp, err)
	}
	users, err := s.getUsersBasedOnBlockNumber(bn, s.mai3GraphInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to get users on block number: blocknumber=%v %w", bn, err)
	}
	s.logger.Debug("found %v users @%v", len(users), bn)
	// 2. update graph data
	prices, err := s.getMarkPrice(bn, s.mai3GraphInterface)
	if err != nil {
		return nil, fmt.Errorf("fail to get mark prices %w", err)
	}
	uis := make([]*mining.UserInfo, len(users))
	for i, u := range users {
		pv, fee, err := s.getOIFeeValue(u.MarginAccounts, bn, prices)
		if err != nil {
			return nil, fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
		}
		// ss is (unlock time - now) * u.StackedMCB <=> s = n * t
		ss := s.getStakeScore(timestamp, u.UnlockMCBTime, u.StakedMCB)
		ui := &mining.UserInfo{
			Trader:        strings.ToLower(u.ID),
			Epoch:         epoch.Epoch,
			CurPosValue:   pv,
			CurStakeScore: ss,
			AccFee:        fee,
		}
		uis[i] = ui
	}
	return uis, nil
}

func (s *Syncer) accumulateCurValues(db *gorm.DB, epoch int64) error {
	if err := db.Model(mining.UserInfo{}).
		Where("epoch=? and cur_pos_value <> 0", epoch).
		UpdateColumn("acc_pos_value", gorm.Expr("acc_pos_value + cur_pos_value")).
		Error; err != nil {
		return fmt.Errorf("failed to accumulate cur_post_value to acc_pos_value  %w", err)
	}
	if err := db.Model(mining.UserInfo{}).
		Where("epoch=? and cur_stake_score <> 0", epoch).
		UpdateColumn("acc_stake_score", gorm.Expr("acc_stake_score + cur_stake_score")).
		Error; err != nil {
		return fmt.Errorf("failed to accumulate cur_stake_score to acc_stake_score %w", err)
	}
	return nil
}

func (s *Syncer) resetCurValues(db *gorm.DB, epoch int64, timestamp int64) error {
	if err := db.Model(mining.UserInfo{}).Where("epoch=?", epoch).
		Updates(mining.UserInfo{CurPosValue: decimal.Zero, CurStakeScore: decimal.Zero, Timestamp: timestamp}).
		Error; err != nil {
		return fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
	}
	return nil
}

func (s *Syncer) updateUserStates(db *gorm.DB, epoch *mining.Schedule, timestamp int64, users []*mining.UserInfo) error {
	if len(users) == 0 {
		return nil
	}
	// acc_pos_value += cur_pos_value if cur_pos_value != 0
	// acc_stake_score += cur_stake_score if cur_stake_score != 0
	if err := s.accumulateCurValues(db, epoch.Epoch); err != nil {
		return fmt.Errorf("failed to accumulate current state %w", err)
	}
	// cur_stake_score <= 0 and cur_pos_value <= 0
	if err := s.resetCurValues(db, epoch.Epoch, timestamp); err != nil {
		return fmt.Errorf("failed to accumulate current state %w", err)
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
		DoUpdates: clause.AssignmentColumns([]string{"cur_pos_value", "cur_stake_score", "acc_fee"}),
	}).Create(&users).Error; err != nil {
		return fmt.Errorf("failed to create user_info: size=%v %w", len(users), err)
	}
	return nil
}

func (s *Syncer) updateUserScores(db *gorm.DB, epoch *mining.Schedule, timestamp int64, users []*mining.UserInfo) error {
	if len(users) == 0 {
		return nil
	}
	var (
		minuteCeil = int64(math.Floor((float64(timestamp) - float64(epoch.StartTime)) / 60.0))
		elapsed    = decimal.NewFromInt(minuteCeil) // Minutes
	)
	for _, ui := range users {
		ui.Score = s.getScore(epoch, ui, elapsed)
	}
	if err := db.Model(&mining.UserInfo{}).Save(&users).Error; err != nil {
		return fmt.Errorf("failed to create user_info: size=%v %w", len(users), err)
	}
	return nil
}

func (s *Syncer) makeSnapshot(db *gorm.DB, timestamp int64, users []*mining.UserInfo) error {
	s.logger.Info("making snapshot for %v", timestamp)
	snapshot := make([]*mining.Snapshot, len(users))
	for i, u := range users {
		snapshot[i] = &mining.Snapshot{
			Trader:        u.Trader,
			Epoch:         u.Epoch,
			Timestamp:     timestamp,
			InitFee:       u.InitFee,
			AccFee:        u.AccFee,
			AccPosValue:   u.AccPosValue,
			CurPosValue:   u.CurPosValue,
			AccStakeScore: u.AccStakeScore,
			CurStakeScore: u.CurStakeScore,
			Score:         u.Score,
		}
	}
	if err := db.Model(&mining.Snapshot{}).Save(&snapshot).Error; err != nil {
		return fmt.Errorf("failed to create snapshot: timestamp=%v, size=%v %w", timestamp, len(users), err)
	}
	return nil
}

func (s *Syncer) syncState(db *gorm.DB, epoch *mining.Schedule) (int64, error) {
	s.logger.Info("enter sync state")
	defer s.logger.Info("leave sync state")

	p, err := s.getProgress(db, PROGRESS_SYNC_STATE, epoch.Epoch)
	if err != nil {
		return 0, fmt.Errorf("fail to get sync progress %w", err)
	}
	var np int64
	if p == 0 {
		np = norm(epoch.StartTime + 60)
	} else {
		np = norm(p + 60)
	}
	newStates, err := s.getUserStateBasedOnBlockNumber(epoch, np)
	if err != nil {
		return 0, fmt.Errorf("fail to get new user states: timestamp=%v %w", np, err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := s.updateUserStates(tx, epoch, np, newStates); err != nil {
			return fmt.Errorf("fail to update current user states: timestamp=%v %w", np, err)
		}
		var allStates []*mining.UserInfo
		if err := tx.Where("epoch=?", epoch.Epoch).Find(&allStates).Error; err != nil {
			return fmt.Errorf("fail to fetch all users in this epoch %w", err)
		}
		if err := s.updateUserScores(tx, epoch, np, allStates); err != nil {
			return fmt.Errorf("fail to update current user states: timestamp=%v %w", np, err)
		}
		if err := s.setProgress(tx, PROGRESS_SYNC_STATE, np, epoch.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		h := normN(np, s.snapshotInterval)
		if np-60 < h && np >= h && len(allStates) > 0 {
			s.logger.Info("making snapshot for %v", h)
			s.makeSnapshot(tx, np, allStates)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("fail to update user states: timestamp=%v %w", np, err)
	}
	return np, nil
}

func (s *Syncer) getStakeScore(curTime int64, unlockTime int64, staked decimal.Decimal) decimal.Decimal {
	if unlockTime < curTime {
		return decimal.Zero
	}
	// floor to 1 if less than 1 day
	days := int64(math.Ceil(float64(unlockTime-curTime) / 86400))
	return decimal.NewFromInt(days).Mul(staked)
}

func (s Syncer) getScore(epoch *mining.Schedule, ui *mining.UserInfo, elapsed decimal.Decimal) decimal.Decimal {
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
	wFee, _ := epoch.WeightFee.Float64()
	wStake, _ := epoch.WeightMCB.Float64()
	wPos, _ := epoch.WeightOI.Float64()
	feeFloat, _ := fee.Float64()
	stakeFloat, _ := stake.Float64()
	posValFloat, _ := posVal.Float64()
	score := math.Pow(feeFloat, wFee) * math.Pow(stakeFloat/elapsedFloat, wStake) * math.Pow(posValFloat/elapsedFloat, wPos)
	return decimal.NewFromFloat(score)
}

func (s *Syncer) getOIFeeValue(accounts []*graph.MarginAccount, bn int64, cache map[string]decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	oi := decimal.Zero
	fee := decimal.Zero
	for _, a := range accounts {
		var price decimal.Decimal

		// 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0-0x00233150044aec4cba478d0bf0ecda0baaf5ad19
		perpId := strings.Join(strings.Split(a.ID, "-")[:2], "-") // 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0
		// inverse contract
		if env.InInverseContractWhiteList(perpId) {
			oi = oi.Add(a.Position.Abs())
			fee = fee.Add(a.InversePoolTotalFee)
			continue
		}

		// normal contract
		if v, ok := cache[perpId]; ok {
			price = v
		} else {
			addr, _, index, err := splitMarginAccountID(a.ID)
			if err != nil {
				return oi, fee, fmt.Errorf("fail to get pool address and index from id %s", err)
			}
			p, err := s.getMarkPriceWithBlockNumberAddrIndex(bn, addr, index, s.mai3GraphInterface)
			if err != nil {
				return oi, fee, fmt.Errorf("fail to get mark price %w", err)
			}
			price = p
			cache[perpId] = p
		}
		oi = oi.Add(price.Mul(a.Position).Abs())
		fee = fee.Add(a.TotalFee)
	}
	return oi, fee, nil
}

func (s *Syncer) detectEpoch(db *gorm.DB, lastTimestamp int64) (*mining.Schedule, error) {
	// start from default epoch start time
	if lastTimestamp == 0 {
		lastTimestamp = s.defaultEpochStartTime
	}
	var ss []*mining.Schedule
	if err := db.Where("end_time>?", lastTimestamp+60).Order("epoch asc").Find(&ss).Error; err != nil {
		return nil, fmt.Errorf("fail to found epoch config %w", err)
	}
	if len(ss) == 0 {
		return nil, ErrNotInEpoch
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
