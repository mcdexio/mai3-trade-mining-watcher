package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// Position defines struct to contain information of open interest
type Position struct {
	ID           int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	Trader       string          `gorm:"column:trader;type:varchar(128);not null" json:"trader"`
	PerpetualAdd string          `gorm:"column:perpetual_add;type:varchar(128);not null" json:"perpetual_add"`
	Position     decimal.Decimal `gorm:"column:position;type:decimal(38,18);not null" json:"position"`
	MarkPrice    decimal.Decimal `gorm:"column:mark_price;type:decimal(38,18);not null" json:"mark_price"`
	EntryValue   decimal.Decimal `gorm:"column:entry_value;type:decimal(38,18);not null" json:"entry_value"`

	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*Position) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*Position) Indexes() []models.CustomIndex {
	return nil
}
