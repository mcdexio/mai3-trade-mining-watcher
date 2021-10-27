package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"go.uber.org/atomic"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/block"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/mai3"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
)

var (
	ErrNotInEpoch = errors.New("not in epoch period")
)

var (
	PROGRESS_SYNC_STATE = "user_info" // compatible
	PROGRESS_INIT_FEE   = "user_info.init_fee"
	PROGRESS_SNAPSHOT   = "user_info.snapshot"
	conflictColumns     = []clause.Column{{Name: "trader"}, {Name: "epoch"}, {Name: "chain"}}
)

type Syncer struct {
	ctx    context.Context
	logger logging.Logger
	db     *gorm.DB

	blockGraphs block.MultiBlockInterface
	mai3Graphs  mai3.MultiGraphInterface

	// default if you don't set epoch in schedule database
	defaultEpochStartTime int64
	syncDelaySeconds      int64

	needRestore      atomic.Bool
	restoreTimestamp int64
	snapshotInterval int64
}

func NewSyncer(
	ctx context.Context, logger logging.Logger, multiMAI3GraphClient *mai3.MultiClient,
	multiBlockGraphClient *block.MultiClient, defaultEpochStartTime int64, syncDelaySeconds int64,
	snapshotInterval int64,
) *Syncer {
	return &Syncer{
		ctx:                   ctx,
		logger:                logger,
		blockGraphs:           multiBlockGraphClient,
		mai3Graphs:            multiMAI3GraphClient,
		db:                    database.GetDB(),
		defaultEpochStartTime: defaultEpochStartTime,
		syncDelaySeconds:      syncDelaySeconds,
		snapshotInterval:      snapshotInterval,
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
	// TODO(champFu): find a way to iterate all fields
	users := make([]*mining.UserInfo, len(snapshots))
	for i, s := range snapshots {
		users[i] = &mining.UserInfo{
			// 13
			Trader:              s.Trader,
			Epoch:               s.Epoch,
			Timestamp:           s.Timestamp,
			InitFee:             s.InitFee,
			AccFee:              s.AccFee,
			InitTotalFee:        s.InitTotalFee,
			AccTotalFee:         s.AccTotalFee,
			AccPosValue:         s.AccPosValue,
			CurPosValue:         s.CurPosValue,
			AccStakeScore:       s.AccStakeScore,
			CurStakeScore:       s.CurStakeScore,
			EstimatedStakeScore: s.EstimatedStakeScore,
			Score:               s.Score,
			Chain:               s.Chain,
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
	startTime := time.Now().Unix()
	s.logger.Info("enter runSync lastTs %d", lastTs)
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
	defer func() {
		endTime := time.Now().Unix()
		s.logger.Info("leave runSync epoch done: epoch=%+v, takes %d seconds",
			epoch, endTime-startTime)
	}()
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
		s.logger.Info("leave initUserState, takes %d seconds", endTime-startTime)
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

	// handle multi-chain
	summaryUser := make(map[string]*mining.UserInfo)
	saveUsers := make([][]*mining.UserInfo, len(multiUsers))
	for i, users := range multiUsers {
		saveUser := make([]*mining.UserInfo, len(users))
		for j, u := range users {
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
					Chain:        "total",
				}
			}
			saveUser[j] = &mining.UserInfo{
				Trader:       userId,
				Epoch:        epoch.Epoch,
				InitFee:      daoFee,
				InitTotalFee: totalFee,
				Chain:        strconv.Itoa(i),
			}
		}
		saveUsers[i] = saveUser
	}

	var uis []*mining.UserInfo
	for _, u := range summaryUser {
		uis = append(uis, u)
	}
	for i, u := range saveUsers {
		s.logger.Info("Network (%d/%d): %d users @BN=%d", i+1, len(saveUsers), len(u), multiBNs[i])
	}
	s.logger.Info("Total users %d @TS=%d", len(uis), epoch.StartTime)

	// update columns if conflict
	updatedColumns := []string{"epoch", "init_fee", "init_total_fee"}
	err = db.Transaction(func(tx *gorm.DB) error {
		// total
		if len(uis) > 0 {
			// len(uis) > 0, because of limitation of postgresql (65535 parameters), do batch
			if err = db.Clauses(clause.OnConflict{
				Columns:   conflictColumns,
				DoUpdates: clause.AssignmentColumns(updatedColumns),
			}).CreateInBatches(&uis, 500).Error; err != nil {
				return fmt.Errorf("fail to create init user info %w", err)
			}
		}

		// handle multi-chain
		for _, u := range saveUsers {
			if len(u) > 0 {
				// because of limitation of postgresql (65535 parameters), do batch
				if err = db.Clauses(clause.OnConflict{
					Columns:   conflictColumns,
					DoUpdates: clause.AssignmentColumns(updatedColumns),
				}).CreateInBatches(&u, 500).Error; err != nil {
					return fmt.Errorf("fail to create init user info %w", err)
				}
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

func (s *Syncer) getUserStateBasedOnBlockNumber(epoch *mining.Schedule, timestamp int64) ([]*mining.UserInfo, [][]*mining.UserInfo, error) {
	multiBNs, multiUsers, multiPrices, err := s.getMultiChainInfo(timestamp)
	if err != nil {
		return nil, nil, err
	}

	// handle multi-chain
	summaryUser := make(map[string]*mining.UserInfo)
	saveUsers := make([][]*mining.UserInfo, len(multiUsers))
	for i, users := range multiUsers {
		saveUser := make([]*mining.UserInfo, len(users))
		for j, u := range users {
			pv, totalFee, daoFee, err := s.getOIFeeValue(u.MarginAccounts, multiBNs[i], multiPrices, i)
			if err != nil {
				return nil, nil, err
			}
			ss := getStakeScore(timestamp, u.UnlockMCBTime, u.StakedMCB)
			estimatedStakeScore := getEstimatedStakeScore(timestamp, epoch, u.UnlockMCBTime, ss)

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
					Chain:               "total",
				}
			}

			saveUser[j] = &mining.UserInfo{
				Trader:              userId,
				Epoch:               epoch.Epoch,
				CurPosValue:         pv,
				CurStakeScore:       ss,
				EstimatedStakeScore: estimatedStakeScore,
				AccFee:              daoFee,
				AccTotalFee:         totalFee,
				Chain:               strconv.Itoa(i),
			}
		}
		saveUsers[i] = saveUser
	}

	var uis []*mining.UserInfo
	for _, u := range summaryUser {
		uis = append(uis, u)
	}
	for i, u := range saveUsers {
		s.logger.Info("Network (%d/%d): %d users @BN=%d", i+1, len(saveUsers), len(u), multiBNs[i])
	}
	s.logger.Info("Total users %d @TS=%d", len(uis), timestamp)
	return uis, saveUsers, nil
}

func (s *Syncer) accumulateCurValuesAllChains(db *gorm.DB, epoch int64) error {
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

func (s *Syncer) resetCurValuesAllChains(db *gorm.DB, epoch int64, timestamp int64) error {
	if err := db.Model(mining.UserInfo{}).Where("epoch=?", epoch).
		Updates(mining.UserInfo{CurPosValue: decimal.Zero, CurStakeScore: decimal.Zero, Timestamp: timestamp}).
		Error; err != nil {
		return fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
	}
	return nil
}

func (s *Syncer) updateUserStates(db *gorm.DB, users []*mining.UserInfo) error {
	if len(users) == 0 {
		return nil
	}
	// update columns if conflict
	updatedColumns := []string{"cur_pos_value", "cur_stake_score", "acc_fee", "acc_total_fee", "estimated_stake_score"}
	// because of limitation of postgresql (65535 parameters), do batch
	if err := db.Clauses(clause.OnConflict{
		Columns:   conflictColumns,
		DoUpdates: clause.AssignmentColumns(updatedColumns),
	}).CreateInBatches(&users, 500).Error; err != nil {
		return fmt.Errorf("failed to create user_info: size=%v %w", len(users), err)
	}

	// make sure acc_fee >= init_fee, acc_total_fee >= init_total_fee
	if err := db.Clauses(clause.OnConflict{
		Columns: conflictColumns,
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
	remains := GetRemainMinutes(timestamp, epoch)

	for _, ui := range users {
		ui.Score = getScore(epoch, ui, remains)
	}
	// update columns if conflict
	updatedColumns := []string{"score"}
	// because of limitation of postgresql (65535 parameters), do batch
	if err := db.Clauses(clause.OnConflict{
		Columns:   conflictColumns,
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
			// 13
			Trader:              u.Trader,
			Epoch:               u.Epoch,
			Timestamp:           timestamp,
			InitFee:             u.InitFee,
			AccFee:              u.AccFee,
			InitTotalFee:        u.InitTotalFee,
			AccTotalFee:         u.AccTotalFee,
			AccPosValue:         u.AccPosValue,
			CurPosValue:         u.CurPosValue,
			AccStakeScore:       u.AccStakeScore,
			CurStakeScore:       u.CurStakeScore,
			EstimatedStakeScore: u.EstimatedStakeScore,
			Score:               u.Score,
			Chain:               u.Chain,
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
		s.logger.Info("leave sync state, takes %d seconds", endTime-startTime)
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
	newStates, saveUsers, err := s.getUserStateBasedOnBlockNumber(epoch, np)
	if err != nil {
		return 0, fmt.Errorf("fail to get new user states: timestamp=%v %w", np, err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		// acc_pos_value += cur_pos_value if cur_pos_value != 0
		// acc_stake_score += cur_stake_score if cur_stake_score != 0
		if err := s.accumulateCurValuesAllChains(db, epoch.Epoch); err != nil {
			return fmt.Errorf("failed to accumulate current state %w", err)
		}
		// cur_stake_score <= 0 and cur_pos_value <= 0
		if err := s.resetCurValuesAllChains(db, epoch.Epoch, np); err != nil {
			return fmt.Errorf("failed to accumulate current state %w", err)
		}

		// update total
		if err := s.updateUserStates(tx, newStates); err != nil {
			return fmt.Errorf("fail to updateUserStates for all: ts=%v, err=%w", np, err)
		}
		// update multi-chains
		countChains := len(saveUsers)
		for chainID := 0; chainID < countChains; chainID++ {
			if err := s.updateUserStates(tx, saveUsers[chainID]); err != nil {
				return fmt.Errorf("fail to updateUserStates for chain %d: ts=%v err=%s", chainID, np, err)
			}
		}

		// calculate for total
		var allStates []*mining.UserInfo
		if err := tx.Where("epoch=? and chain=?", epoch.Epoch, "total").Find(&allStates).Error; err != nil {
			return fmt.Errorf("fail to fetch all users in this epoch err=%s", err)
		}
		if err := s.updateUserScores(tx, epoch, np, allStates); err != nil {
			return fmt.Errorf("fail to updateUserScores for all: ts=%v err=%s", np, err)
		}

		// calculate for multi-chains
		for chainID := 0; chainID < countChains; chainID++ {
			var states []*mining.UserInfo
			if err := tx.Where("epoch=? and chain=?", epoch.Epoch, strconv.Itoa(chainID)).Find(&states).Error; err != nil {
				return fmt.Errorf("fail to fetch users for chain %d in this epoch err=%s", chainID, err)
			}
			if err := s.updateUserScores(tx, epoch, np, states); err != nil {
				return fmt.Errorf("fail to updateUserScores for chain %d: ts=%v err=%s", chainID, np, err)
			}
		}

		if err := s.setProgress(tx, PROGRESS_SYNC_STATE, np, epoch.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		h := normN(np, s.snapshotInterval)
		if np-60 < h && np >= h && len(allStates) > 0 {
			s.logger.Info("making snapshot for %v", h)
			err := s.makeSnapshot(tx, np, allStates)
			if err != nil {
				s.logger.Error("err=%s", err)
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("fail to update user states: timestamp=%v %w", np, err)
	}
	return np, nil
}

func (s *Syncer) getOIFeeValue(
	accounts []*mai3.MarginAccount, blockNumbers int64, cache map[string]decimal.Decimal,
	mai3GraphIndex int) (oi, totalFee, daoFee decimal.Decimal, err error) {
	mai3Graph, err := s.mai3Graphs.GetMai3GraphInterface(mai3GraphIndex)
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
		base := ""
		// is BTC inverse contract
		match, base = mai3Graph.InBTCInverseContractWhiteList(perpId)
		if match {
			var btcPerpetualID, basePerpetualID string

			btcPerpetualID, err = mai3Graph.GetPerpIDWithUSDBased("BTC")
			if err != nil {
				return
			}
			totalFee = totalFee.Add(a.TotalFee.Mul(cache[btcPerpetualID]))
			dFee := a.OperatorFee.Add(a.VaultFee)
			daoFee = daoFee.Add(dFee.Mul(cache[btcPerpetualID]))

			if base == "USD" {
				oi = oi.Add(a.Position.Abs())
				continue
			}
			// base not USD
			basePerpetualID, err = mai3Graph.GetPerpIDWithUSDBased(base)
			if err != nil {
				return
			}
			oi = oi.Add(a.Position.Abs().Mul(cache[basePerpetualID]))
			continue
		}
		// is ETH inverse contract
		match, base = mai3Graph.InETHInverseContractWhiteList(perpId)
		if match {
			var ethPerpetualID, basePerpetualID string

			ethPerpetualID, err = mai3Graph.GetPerpIDWithUSDBased("ETH")
			if err != nil {
				return
			}
			totalFee = totalFee.Add(a.TotalFee.Mul(cache[ethPerpetualID]))
			dFee := a.OperatorFee.Add(a.VaultFee)
			daoFee = daoFee.Add(dFee.Mul(cache[ethPerpetualID]))

			if base == "USD" {
				oi = oi.Add(a.Position.Abs())
				continue
			}
			// quote not USD
			basePerpetualID, err = mai3Graph.GetPerpIDWithUSDBased(base)
			if err != nil {
				return
			}
			oi = oi.Add(a.Position.Abs().Mul(cache[basePerpetualID]))
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
	blockNumbers []int64, mai3Graphs mai3.MultiGraphInterface) ([][]mai3.User, error) {
	return mai3Graphs.GetMultiUsersBasedOnMultiBlockNumbers(blockNumbers)
}

// getMarkPrices the order of mai3Graphs need to match blockNumbers
func (s *Syncer) getMultiMarkPrices(
	blockNumbers []int64, mai3Graphs mai3.MultiGraphInterface) (map[string]decimal.Decimal, error) {
	return mai3Graphs.GetMultiMarkPrices(blockNumbers)
}

func (s *Syncer) getMarkPriceWithBlockNumberAddrIndex(
	blockNumbers int64, poolAddr string, perpIndex int, mai3Graph mai3.GraphInterface) (
	decimal.Decimal, error) {
	return mai3Graph.GetMarkPriceWithBlockNumberAddrIndex(blockNumbers, poolAddr, perpIndex)
}

func (s *Syncer) getMultiBlockNumberWithTS(
	timestamp int64, blockGraphs block.MultiBlockInterface) ([]int64, error) {
	return blockGraphs.GetMultiBlockNumberWithTS(timestamp)
}

func (s *Syncer) getMultiChainInfo(timestamp int64) (
	multiBNs []int64, multiUsers [][]mai3.User, multiPrices map[string]decimal.Decimal, err error) {
	multiBNs, err = s.getMultiBlockNumberWithTS(timestamp, s.blockGraphs)
	if err != nil {
		return
	}
	multiUsers, err = s.getMultiChainUsersBasedOnBlockNumber(multiBNs, s.mai3Graphs)
	if err != nil {
		return
	}
	multiPrices, err = s.getMultiMarkPrices(multiBNs, s.mai3Graphs)
	if err != nil {
		return
	}
	return
}
