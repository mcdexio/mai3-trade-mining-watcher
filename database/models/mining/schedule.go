package mining

import "github.com/mcdexio/mai3-trade-mining-watcher/database/models"

type Schedule struct {
	ID        string `gorm:"column:id;type:varchar(129);not null;primary_key" json:"id"`
	Epoch     int64  `gorm:"column:epoch;type:bigint;not null" json:"epoch"`
	From      int64  `gorm:"column:from;type:bigint;not null" json:"from"`
	To        int64  `gorm:"column:to;type:bigint;not null" json:"to"`
	WeightFee int64  `gorm:"column:weight_fee;type:bigint;not null" json:"weight_fee"`
	WeightOI  int64  `gorm:"column:weight_oi;type:bigint;not null" json:"weight_oi"`
	WeightMCB int64  `gorm:"column:weight_mcb;type:bigint;not null" json:"weight_mcb"`
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
