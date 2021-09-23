package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// Stack defines struct to contain information of open interest
type Stack struct {
	ID      int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	UserAdd string          `gorm:"column:user_add;type:varchar(128);not null" json:"user_add"`
	Stack   decimal.Decimal `gorm:"column:stack;type:decimal(38,18);not null" json:"stack"`

	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*Stack) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*Stack) Indexes() []models.CustomIndex {
	return nil
}
