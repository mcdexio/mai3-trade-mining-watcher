package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// Fee defines struct to contain information of fee
type Fee struct {
	ID     int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	Trader string          `gorm:"column:trader;type:varchar(128);not null" json:"trader"`
	Fee    decimal.Decimal `gorm:"column:fee;type:decimal(38,18);not null" json:"fee"`

	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*Fee) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*Fee) Indexes() []models.CustomIndex {
	return nil
}
