package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// Perpetual defines struct to contain information of a perpetual.
type Perpetual struct {
	models.Base

	ID             int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null" json:"id"`
	PerpetualIndex int64           `gorm:"column:perpetual_index;type;bigint;not null" json:"perpetual_index"`
	MarkPrice      decimal.Decimal `gorm:"column:mark_price;type:decimal(38,18);not null" json:"mark_price"`

	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`
}

// Indexes returns information to create index.
func (*Perpetual) Indexes() []models.CustomIndex {
	return nil
}
