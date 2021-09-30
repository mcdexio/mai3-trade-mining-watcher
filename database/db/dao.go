package db

import (
	"errors"
	"fmt"

	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotInEpoch = errors.New("not in epoch period")

var (
	ProgressSyncStateID = "user_info" // compatible
	ProgressInitFeeID   = "user_info.init_fee"
)

type DAO struct {
	ProgressDAO
	UserInfoDAO
}

type ProgressDAO struct {
}

func (pd *ProgressDAO) GetInitProgress(db *gorm.DB, epoch int64) (int64, error) {
	return pd.getProgress(db, ProgressInitFeeID, epoch)
}

func (pd *ProgressDAO) SaveInitProgress(db *gorm.DB, timestamp, epoch int64) error {
	return pd.setProgress(db, ProgressInitFeeID, timestamp, epoch)
}

func (pd *ProgressDAO) GetSyncProgress(db *gorm.DB, epoch int64) (int64, error) {
	return pd.getProgress(db, ProgressSyncStateID, epoch)
}

func (pd *ProgressDAO) SaveSyncProgress(db *gorm.DB, timestamp, epoch int64) error {
	return pd.setProgress(db, ProgressSyncStateID, timestamp, epoch)
}

func (pd *ProgressDAO) DetectEpoch(db *gorm.DB, fallbackEpoch int64) (*mining.Schedule, error) {
	p, err := pd.getLastProgress(db)
	if err != nil {
		return nil, err
	}
	// 0 or non-zero
	if p == 0 {
		var s *mining.Schedule
		if err := db.Where("epoch=?", fallbackEpoch).First(&s).Error; err != nil {
			return nil, fmt.Errorf("fail to found epoch config %w", err)
		}
		return s, nil
	}
	var ss []*mining.Schedule
	if err := db.Where("end_time>?", p+60).Order("epoch asc").Find(&ss).Error; err != nil {
		return nil, fmt.Errorf("fail to found epoch config %w", err)
	}
	if len(ss) == 0 {
		return nil, ErrNotInEpoch
	}
	return ss[0], nil
}

func (pd *ProgressDAO) getLastProgress(db *gorm.DB) (int64, error) {
	var p mining.Progress
	err := db.Where("table_name=?", ProgressSyncStateID).Order("epoch desc").First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get last progress %w", err)
	}
	return p.From, nil
}

func (pd *ProgressDAO) getProgress(db *gorm.DB, name string, epoch int64) (int64, error) {
	p := &mining.Progress{}
	err := db.Where("table_name=? and epoch=?", name, epoch).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get progress: table=%s %w", name, err)
	}
	return p.From, nil
}

func (pd *ProgressDAO) setProgress(db *gorm.DB, name string, ts int64, epoch int64) error {
	return db.Save(&mining.Progress{
		TableName: types.TableName(name),
		From:      ts,
		Epoch:     epoch,
	}).Error
}

type UserInfoDAO struct {
}

func (uid *UserInfoDAO) GetAllUserInfo(db *gorm.DB, epoch int64) ([]*mining.UserInfo, error) {
	var all []*mining.UserInfo
	if err := db.Where("epoch=?", epoch).Find(&all).Error; err != nil {
		return nil, fmt.Errorf("fail to fetch all users in this epoch %w", err)
	}
	return all, nil
}

func (uid *UserInfoDAO) AccumulateCurValues(db *gorm.DB, epoch int64) error {
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

func (uid *UserInfoDAO) ResetCurValues(db *gorm.DB, epoch int64, timestamp int64) error {
	if err := db.Model(mining.UserInfo{}).Where("epoch=?", epoch).
		Updates(mining.UserInfo{CurPosValue: decimal.Zero, CurStakeScore: decimal.Zero, Timestamp: timestamp}).
		Error; err != nil {
		return fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
	}
	return nil
}

func (uid *UserInfoDAO) UpdateUserInfo(db *gorm.DB, userInfos []*mining.UserInfo) error {
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
		DoUpdates: clause.AssignmentColumns([]string{"cur_pos_value", "cur_stake_score", "acc_fee"}),
	}).Create(&userInfos).Error; err != nil {
		return fmt.Errorf("failed to create user_info: size=%v %w", len(userInfos), err)
	}
	return nil
}
