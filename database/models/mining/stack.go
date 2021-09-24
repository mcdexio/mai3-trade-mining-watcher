package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// Stake defines struct to contain information of open interest
type Stake struct {
	ID     int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	Trader string          `gorm:"column:trader;type:varchar(128);not null" json:"trader"`
	Stake  decimal.Decimal `gorm:"column:stake;type:decimal(38,18);not null" json:"stake"`

	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*Stake) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*Stake) Indexes() []models.CustomIndex {
	return nil
}
