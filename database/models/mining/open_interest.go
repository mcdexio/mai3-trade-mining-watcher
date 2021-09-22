package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// OpenInterest defines struct to contain information of open interest
type OpenInterest struct {
	ID             int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	User           string          `gorm:"column:user;type:varchar(128);not null" json:"user"`
	PerpetualIndex int64           `gorm:"column:perpetual_index;type:bigint;not null" json:"perpetual_index"`
	Position       decimal.Decimal `gorm:"column:position;type:decimal(38,18);not null" json:"position"`
	MarkPrice      decimal.Decimal `gorm:"column:mark_price;type:decimal(38,18);not null" json:"mark_price"`
	OI             decimal.Decimal `gorm:"column:oi;type:decimal(38,18);not null" json:"oi"`

	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*OpenInterest) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*OpenInterest) Indexes() []models.CustomIndex {
	return nil
}
