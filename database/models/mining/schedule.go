package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

type Schedule struct {
	Epoch     int64           `gorm:"column:epoch;type:bigint;not null" json:"epoch"`
	From      int64           `gorm:"column:from;type:bigint;not null" json:"from"`
	To        int64           `gorm:"column:to;type:bigint;not null" json:"to"`
	WeightFee decimal.Decimal `gorm:"column:weight_fee;type:decimal(38,18);not null" json:"weight_fee"`
	WeightOI  decimal.Decimal `gorm:"column:weight_oi;type:decimal(38,18);not null" json:"weight_oi"`
	WeightMCB decimal.Decimal `gorm:"column:weight_mcb;type:decimal(38,18);not null" json:"weight_mcb"`
}

// ForeignKeyConstraints create foreign key constraints.
func (*Schedule) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*Schedule) Indexes() []models.CustomIndex {
	return []models.CustomIndex{
		{
			Name:      "schedule_epoch_unique_idx",
			Unique:    true,
			Fields:    []string{"epoch"},
			Type:      "",
			Condition: "",
		},
	}
}
