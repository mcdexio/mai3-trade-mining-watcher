package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// UserInfo defines struct to contain information of a user info
type UserInfo struct {
	Trader    string `gorm:"column:trader;primary_key;type:varchar(128);not null" json:"trader"`
	Epoch     int64  `gorm:"column:epoch;primary_key;type:bigint;not null" json:"epoch"`
	Chain     string `gorm:"column:chain;primary_key;type:varchar(128);" json:"chain"`
	Timestamp int64  `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	InitFee             decimal.Decimal `gorm:"column:init_fee;type:decimal(38,18);not null" json:"init_fee"`
	AccFee              decimal.Decimal `gorm:"column:acc_fee;type:decimal(38,18);not null" json:"acc_fee"`
	InitTotalFee        decimal.Decimal `gorm:"column:init_total_fee;type:decimal(38,18);not null;default:0" json:"init_total_fee"`
	AccTotalFee         decimal.Decimal `gorm:"column:acc_total_fee;type:decimal(38,18);not null;default:0" json:"acc_total_fee"`
	AccPosValue         decimal.Decimal `gorm:"column:acc_pos_value;type:decimal(38,18);not null" json:"acc_pos_value"`
	CurPosValue         decimal.Decimal `gorm:"column:cur_pos_value;type:decimal(38,18);not null" json:"cur_pos_value"`
	AccStakeScore       decimal.Decimal `gorm:"column:acc_stake_score;type:decimal(38,18);not null" json:"acc_stake_score"`
	CurStakeScore       decimal.Decimal `gorm:"column:cur_stake_score;type:decimal(38,18);not null" json:"cur_stake_score"`
	EstimatedStakeScore decimal.Decimal `gorm:"column:estimated_stake_score;type:decimal(38,18);not null" json:"estimated_stake_score"`
	Score               decimal.Decimal `gorm:"column:score;type:decimal(38,18);not null" json:"score"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*UserInfo) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*UserInfo) Indexes() []models.CustomIndex {
	return []models.CustomIndex{
		{
			Name: "trader_epoch_chain_unique_idx",
			Unique: true,
			Fields: []string{"trader", "epoch", "chain"},
			Type: "",
			Condition: "",
		},
	}
}
