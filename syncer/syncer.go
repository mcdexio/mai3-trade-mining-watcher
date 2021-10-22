package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/block"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/mai3"
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
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
)

var (
	ErrNotInEpoch              = errors.New("not in epoch period")
	ErrBlockLengthNotEqualMAI3 = errors.New("length of block graph not equal to mai3 graph")
)

var (
	PROGRESS_SYNC_STATE = "user_info" // compatible
	PROGRESS_INIT_FEE   = "user_info.init_fee"
	PROGRESS_SNAPSHOT   = "user_info.snapshot"
)

type Syncer struct {
	ctx    context.Context
	logger logging.Logger
	db     *gorm.DB

	// block graph
	blockGraph1 block.Interface
	blockGraph2 block.Interface

	// MAI3 graph
	mai3Graph1 mai3.Interface
	mai3Graph2 mai3.Interface

	// default if you don't set epoch in schedule database
	defaultEpochStartTime int64
	syncDelaySeconds      int64

	needRestore      atomic.Bool
	restoreTimestamp int64
	snapshotInterval int64
}

func NewSyncer(
	ctx context.Context, logger logging.Logger, mai3GraphUrlBsc, mai3GraphUrlArb, blockGraphUrlBsc, blockGraphUrlArb string, defaultEpochStartTime int64, syncDelaySeconds int64) *Syncer {
	return &Syncer{
		ctx:                   ctx,
		logger:                logger,
		blockGraph1:           block.NewClient(logger, blockGraphUrlBsc),
		blockGraph2:           block.NewClient(logger, blockGraphUrlArb),
		mai3Graph1:            mai3.NewClient(logger, mai3GraphUrlBsc),
		mai3Graph2:            mai3.NewClient(logger, mai3GraphUrlArb),
		db:                    database.GetDB(),
		defaultEpochStartTime: defaultEpochStartTime,
		syncDelaySeconds:      syncDelaySeconds,
		snapshotInterval:      3600,
	}
}

func (s *Syncer) Run() error {
	for {
		var err error
		switch s.needRestore.Load() {
		case true:
			err = s.runRestore(s.ctx, s.restoreTimestamp)
			if err == nil {
				s.needRestore.Store(false)
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
			InitTotalFee:  s.InitTotalFee,
			AccTotalFee:   s.AccTotalFee,
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
	s.logger.Debug("runSync lastTs %d", lastTs)
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
	s.logger.Debug("enter initUserStates epoch %d", epoch.Epoch)
	startTime := time.Now().Unix()
	defer func() {
		endTime := time.Now().Unix()
		s.logger.Info("leave initUserState, takes %d second for initUserState", endTime-startTime)
	}()

	p, err := s.getProgress(db, PROGRESS_INIT_FEE, epoch.Epoch)
	if err != nil {
		return fmt.Errorf("fail to get sync progress %w", err)
	}
	// already synced
	if p != 0 && p == epoch.StartTime {
		s.logger.Info("fee already initialized")
		return nil
	}

	multiBNs, multiUsers, multiPrices, err := s.getMultiChainInfo(epoch.StartTime)
	if err != nil {
		return err
	}

	summaryUser := make(map[string]*mining.UserInfo)
	// handle multi-chain
	for i, users := range multiUsers {
		for _, u := range users {
			_, totalFee, daoFee, err := s.getOIFeeValue(u.MarginAccounts, multiBNs[i], multiPrices, i)
			if err != nil {
				return fmt.Errorf("fail to get initial fee %s", err)
			}

			userId := strings.ToLower(u.ID)

			if user, match := summaryUser[userId]; match {
				summaryUser[userId].InitFee = user.InitFee.Add(daoFee)
				summaryUser[userId].InitTotalFee = user.InitTotalFee.Add(totalFee)
			} else {
				summaryUser[userId] = &mining.UserInfo{
					Trader:       userId,
					Epoch:        epoch.Epoch,
					InitFee:      daoFee,
					InitTotalFee: totalFee,
				}
			}
		}
	}

	var uis []*mining.UserInfo
	for _, u := range summaryUser {
		uis = append(uis, u)
	}
	for i, u := range multiUsers {
		s.logger.Info("Network (%d/%d): %d users @BN=%d", i, len(multiUsers), len(u), multiBNs[i])
	}
	s.logger.Info("Total users %d @TS=%d", len(uis), epoch.StartTime)

	// update columns if conflict
	updatedColumns := []string{"epoch", "init_fee", "init_total_fee"}
	err = db.Transaction(func(tx *gorm.DB) error {
		lengthUis := len(uis)
		if lengthUis > 0 {
			// len(uis) > 0, because of limitation of postgresql (65535 parameters), do batch
			if err = db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
				DoUpdates: clause.AssignmentColumns(updatedColumns),
			}).CreateInBatches(&uis, 500).Error; err != nil {
				return fmt.Errorf("fail to create init user info %w", err)
			}
		}
		if err = s.setProgress(db, PROGRESS_INIT_FEE, epoch.StartTime, epoch.Epoch); err != nil {
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
	multiBNs, multiUsers, multiPrices, err := s.getMultiChainInfo(timestamp)
	if err != nil {
		return nil, err
	}
	summaryUser := make(map[string]*mining.UserInfo)
	// handle multi-chain
	for i, users := range multiUsers {
		for _, u := range users {
			pv, totalFee, daoFee, err := s.getOIFeeValue(u.MarginAccounts, multiBNs[i], multiPrices, i)
			if err != nil {
				return nil, err
			}
			ss := s.getStakeScore(timestamp, u.UnlockMCBTime, u.StakedMCB)
			estimatedStakeScore := s.getEstimatedStakeScore(timestamp, epoch, u.UnlockMCBTime, ss)

			userId := strings.ToLower(u.ID)

			if user, match := summaryUser[userId]; match {
				summaryUser[userId].CurPosValue = user.CurPosValue.Add(pv)
				summaryUser[userId].CurStakeScore = user.CurStakeScore.Add(ss)
				summaryUser[userId].EstimatedStakeScore = user.EstimatedStakeScore.Add(estimatedStakeScore)
				summaryUser[userId].AccFee = user.AccFee.Add(daoFee)
				summaryUser[userId].AccTotalFee = user.AccTotalFee.Add(totalFee)
			} else {
				summaryUser[userId] = &mining.UserInfo{
					Trader:              userId,
					Epoch:               epoch.Epoch,
					CurPosValue:         pv,
					CurStakeScore:       ss,
					EstimatedStakeScore: estimatedStakeScore,
					AccFee:              daoFee,
					AccTotalFee:         totalFee,
				}
			}
		}
	}

	var uis []*mining.UserInfo
	for _, u := range summaryUser {
		uis = append(uis, u)
	}
	for i, u := range multiUsers {
		s.logger.Info("Network (%d/%d): %d users @BN=%d", i, len(multiUsers), len(u), multiBNs[i])
	}
	s.logger.Info("Total users %d @TS=%d", len(uis), timestamp)
	return uis, nil
}

func (s *Syncer) getEstimatedStakeScore(
	nowTimestamp int64, epoch *mining.Schedule, unlockTime int64,
	currentStakingReward decimal.Decimal,
) decimal.Decimal {
	// A = (1 - Floor(RemainEpochSeconds / 86400) / UnlockTimeInDays / 2) * CurrentStakingReward * RemainEpochMinutes
	// EstimatedAverageStakingScore  = (CumulativeStakingScore + A) / TotalEpochMinutes

	// floor to 0 if less than 1 day
	remainEpochDays := math.Floor(float64(epoch.EndTime-nowTimestamp) / 86400)
	// fmt.Printf("remainEpochDays %v\n", remainEpochDays)
	// ceil to 1 if less than 1 day
	unlockTimeInDays := math.Ceil(float64(unlockTime-nowTimestamp) / 86400)
	// fmt.Printf("unlockTimeInDays %v\n", unlockTimeInDays)
	remainProportion := decimal.NewFromFloat(1.0 - (remainEpochDays / unlockTimeInDays / 2.0))
	// fmt.Printf("remainProportion %v\n", remainProportion)
	// ceil to 1 if less than 1 minute
	remainEpochMinutes := decimal.NewFromFloat(math.Ceil(float64(epoch.EndTime-nowTimestamp) / 60))
	// fmt.Printf("remainEpochMinutes %v\n", remainEpochMinutes)

	return remainProportion.Mul(currentStakingReward).Mul(remainEpochMinutes)
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

	// update columns if conflict
	updatedColumns := []string{"cur_pos_value", "cur_stake_score", "acc_fee", "acc_total_fee", "estimated_stake_score"}
	// because of limitation of postgresql (65535 parameters), do batch
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
		DoUpdates: clause.AssignmentColumns(updatedColumns),
	}).CreateInBatches(&users, 500).Error; err != nil {
		return fmt.Errorf("failed to create user_info: size=%v %w", len(users), err)
	}

	// make sure acc_fee >= init_fee, acc_total_fee >= init_total_fee
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "trader"}, {Name: "epoch"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"acc_fee":       gorm.Expr("GREATEST(user_info.acc_fee, user_info.init_fee)"),
			"acc_total_fee": gorm.Expr("GREATEST(user_info.acc_total_fee, user_info.init_total_fee)"),
		}),
	}).CreateInBatches(&users, 1000).Error; err != nil {
		return fmt.Errorf("failed to max(acc_fee, init_fee): size=%v %w", len(users), err)
	}
	return nil
}

func (s *Syncer) updateUserScores(db *gorm.DB, epoch *mining.Schedule, timestamp int64, users []*mining.UserInfo) error {
	if len(users) == 0 {
		return nil
	}
	var (
		minuteCeil = int64(math.Floor((float64(timestamp) - float64(epoch.StartTime)) / 60.0))
		remains    = decimal.NewFromInt((epoch.EndTime-epoch.StartTime)/60.0 - minuteCeil) // total epoch in minutes
	)
	for _, ui := range users {
		ui.Score = s.getScore(epoch, ui, remains)
	}
	// update columns if conflict
	updatedColumns := []string{"score"}
	// because of limitation of postgresql (65535 parameters), do batch
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
		DoUpdates: clause.AssignmentColumns(updatedColumns),
	}).CreateInBatches(&users, 500).Error; err != nil {
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
			InitTotalFee:  u.InitTotalFee,
			AccTotalFee:   u.AccTotalFee,
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
	s.logger.Info("enter sync state epoch %d", epoch.Epoch)
	startTime := time.Now().Unix()
	defer func() {
		endTime := time.Now().Unix()
		s.logger.Info("leave sync state, takes %d second for syncState", endTime-startTime)
	}()

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
			return fmt.Errorf("fail to updateUserStates: timestamp=%v %w", np, err)
		}
		var allStates []*mining.UserInfo
		if err := tx.Where("epoch=?", epoch.Epoch).Find(&allStates).Error; err != nil {
			return fmt.Errorf("fail to fetch all users in this epoch %w", err)
		}
		if err := s.updateUserScores(tx, epoch, np, allStates); err != nil {
			return fmt.Errorf("fail to updateUserScores: timestamp=%v %w", np, err)
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
	// ss is (unlock time - now) * u.StackedMCB <=> s = n * t
	if unlockTime < curTime {
		return decimal.Zero
	}
	// floor to 1 if less than 1 day
	days := int64(math.Ceil(float64(unlockTime-curTime) / 86400))
	return decimal.NewFromInt(days).Mul(staked)
}

func (s Syncer) getScore(epoch *mining.Schedule, ui *mining.UserInfo, remains decimal.Decimal) decimal.Decimal {
	if ui.AccTotalFee.IsZero() {
		return decimal.Zero
	}
	fee := decimal.Zero
	if epoch.Epoch == 0 {
		// epoch 0 is totalFee
		fee = ui.AccTotalFee.Sub(ui.InitTotalFee)
	} else {
		fee = ui.AccFee.Sub(ui.InitFee)
	}
	if fee.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	stake := ui.AccStakeScore.Add(ui.CurStakeScore)
	stake = stake.Add(ui.EstimatedStakeScore)
	if stake.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	// EstimatedOpenInterest = (CumulativeOpenInterest + CurrentOpenInterest * RemainEpochMinutes) / TotalEpochMinutes
	posVal := ui.AccPosValue.Add(ui.CurPosValue.Mul(remains))
	if posVal.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	// ceil to 1 if less than 1 minute
	totalEpochMinutes := math.Ceil(float64(epoch.EndTime-epoch.StartTime) / 60)

	// decimal package has issue on pow function
	wFee, _ := epoch.WeightFee.Float64()
	wStake, _ := epoch.WeightMCB.Float64()
	wPos, _ := epoch.WeightOI.Float64()
	feeFloat, _ := fee.Float64()
	stakeFloat, _ := stake.Float64()
	posValFloat, _ := posVal.Float64()
	score := math.Pow(feeFloat, wFee) * math.Pow(stakeFloat/totalEpochMinutes, wStake) * math.Pow(posValFloat/totalEpochMinutes, wPos)
	if math.IsNaN(score) {
		return decimal.Zero
	}
	return decimal.NewFromFloat(score)
}

func (s *Syncer) getOIFeeValue(
	accounts []*mai3.MarginAccount, blockNumbers int64, cache map[string]decimal.Decimal,
	mai3GraphIndex int) (oi, totalFee, daoFee decimal.Decimal, err error) {
	mai3Graph, err := s.getMai3GraphInterface(mai3GraphIndex)
	if err != nil {
		return
	}
	oi = decimal.Zero
	totalFee = decimal.Zero
	daoFee = decimal.Zero
	for _, a := range accounts {
		var price decimal.Decimal
		var poolAddr string
		var perpIndex int

		// 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0-0x00233150044aec4cba478d0bf0ecda0baaf5ad19
		// perpId := strings.Join(strings.Split(a.ID, "-")[:2], "-")
		poolAddr, _, perpIndex, err = splitMarginAccountID(a.ID)
		if err != nil {
			return
		}
		perpId := fmt.Sprintf("%s-%d", poolAddr, perpIndex) // 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0

		match := false
		quote := ""
		// is BTC inverse contract
		match, quote = env.InBTCInverseContractWhiteList(perpId)
		if match {
			var btcPerpetualID, quotePerpetualID string

			btcPerpetualID, err = env.GetPerpIDWithUSDBased("BTC", mai3GraphIndex)
			if err != nil {
				return
			}
			totalFee = totalFee.Add(a.TotalFee.Mul(cache[btcPerpetualID]))
			dFee := a.OperatorFee.Add(a.VaultFee)
			daoFee = daoFee.Add(dFee.Mul(cache[btcPerpetualID]))

			if quote == "USD" {
				oi = oi.Add(a.Position.Abs())
				continue
			}
			// quote not USD
			quotePerpetualID, err = env.GetPerpIDWithUSDBased(quote, mai3GraphIndex)
			if err != nil {
				return
			}
			oi = oi.Add(a.Position.Abs().Mul(cache[quotePerpetualID]))
			continue
		}
		// is ETH inverse contract
		match, quote = env.InETHInverseContractWhiteList(perpId)
		if match {
			var ethPerpetualID, quotePerpetualID string

			ethPerpetualID, err = env.GetPerpIDWithUSDBased("ETH", mai3GraphIndex)
			if err != nil {
				return
			}
			totalFee = totalFee.Add(a.TotalFee.Mul(cache[ethPerpetualID]))
			dFee := a.OperatorFee.Add(a.VaultFee)
			daoFee = daoFee.Add(dFee.Mul(cache[ethPerpetualID]))

			if quote == "USD" {
				oi = oi.Add(a.Position.Abs())
				continue
			}
			// quote not USD
			quotePerpetualID, err = env.GetPerpIDWithUSDBased(quote, mai3GraphIndex)
			if err != nil {
				return
			}
			oi = oi.Add(a.Position.Abs().Mul(cache[quotePerpetualID]))
			continue
		}

		// normal contract
		if v, ok := cache[perpId]; ok {
			price = v
		} else {
			var p decimal.Decimal
			p, err = s.getMarkPriceWithBlockNumberAddrIndex(blockNumbers, poolAddr, perpIndex, mai3Graph)
			if err != nil {
				return
			}
			price = p
			cache[perpId] = p
		}
		oi = oi.Add(price.Mul(a.Position).Abs())
		totalFee = totalFee.Add(a.TotalFee)
		dFee := a.OperatorFee.Add(a.VaultFee)
		daoFee = daoFee.Add(dFee)
	}
	err = nil
	return
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

// getMultiChainUsersBasedOnBlockNumber the order of mai3Graphs need to match blockNumbers, return 2-D users
func (s *Syncer) getMultiChainUsersBasedOnBlockNumber(
	blockNumbers []int64, mai3Graphs ...mai3.Interface) ([][]mai3.User, error) {
	if len(blockNumbers) != len(mai3Graphs) {
		return nil, ErrBlockLengthNotEqualMAI3
	}

	ret := make([][]mai3.User, len(blockNumbers))
	for i, bn := range blockNumbers {
		users, err := mai3Graphs[i].GetUsersBasedOnBlockNumber(bn)
		if err != nil {
			return nil, err
		}
		ret[i] = users
	}
	return ret, nil
}

// getMarkPrices the order of mai3Graphs need to match blockNumbers
func (s *Syncer) getMultiMarkPrices(blockNumbers []int64, mai3Graphs ...mai3.Interface) (map[string]decimal.Decimal, error) {
	if len(blockNumbers) != len(mai3Graphs) {
		return nil, ErrBlockLengthNotEqualMAI3
	}
	ret := make(map[string]decimal.Decimal)
	for i, bn := range blockNumbers {
		prices, err := mai3Graphs[i].GetMarkPrices(bn)
		if err != nil {
			return nil, err
		}
		for k, v := range prices {
			ret[k] = v
		}
	}
	return ret, nil
}

func (s *Syncer) getMarkPriceWithBlockNumberAddrIndex(
	blockNumbers int64, poolAddr string, perpIndex int, mai3Graph mai3.Interface) (decimal.Decimal, error) {
	return mai3Graph.GetMarkPriceWithBlockNumberAddrIndex(blockNumbers, poolAddr, perpIndex)
}

func (s *Syncer) getMultiBlockNumberWithTS(timestamp int64, blockGraphs ...block.Interface) (
	[]int64, error) {
	var ret []int64
	for _, blockGraph := range blockGraphs {
		bn, err := blockGraph.GetBlockNumberWithTS(timestamp)
		if err != nil {
			return nil, err
		}
		ret = append(ret, bn)
	}
	return ret, nil
}

func (s *Syncer) getMai3GraphInterface(index int) (mai3.Interface, error) {
	if index == 0 {
		return s.mai3Graph1, nil
	} else if index == 1 {
		return s.mai3Graph2, nil
	} else {
		return nil, fmt.Errorf("fail to getMai3GraphInterface index %d", index)
	}
}

func (s *Syncer) getMultiChainInfo(timestamp int64) (
	multiBNs []int64, multiUsers [][]mai3.User, multiPrices map[string]decimal.Decimal, err error) {
	multiBNs, err = s.getMultiBlockNumberWithTS(timestamp, s.blockGraph1, s.blockGraph2)
	if err != nil {
		return
	}
	multiUsers, err = s.getMultiChainUsersBasedOnBlockNumber(multiBNs, s.mai3Graph1, s.mai3Graph2)
	if err != nil {
		return
	}
	multiPrices, err = s.getMultiMarkPrices(multiBNs, s.mai3Graph1, s.mai3Graph2)
	if err != nil {
		return
	}
	return
}
