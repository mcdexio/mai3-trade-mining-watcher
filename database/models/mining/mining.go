package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// Mining defines struct to contain information of a user
type Mining struct {
	ID       int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	User     string          `gorm:"column:user;type:varchar(128);not null" json:"user"`
	Position decimal.Decimal `gorm:"column:position;type:decimal(38,18);not null" json:"position"`
	PerpetualIndex int64           `gorm:"column:perpetual_index;type:bigint;not null" json:"perpetual_index"`
	MarkPrice      decimal.Decimal `gorm:"column:mark_price;type:decimal(38,18);not null" json:"mark_price"`
	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*Mining) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*Mining) Indexes() []models.CustomIndex {
	return nil
}
