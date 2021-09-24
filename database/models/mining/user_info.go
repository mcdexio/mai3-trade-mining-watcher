package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// UserInfo defines struct to contain information of a user info
type UserInfo struct {
	ID      int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	UserAdd string          `gorm:"column:user_add;type:varchar(128);not null" json:"user_add"`
	Fee     decimal.Decimal `gorm:"column:fee;type:decimal(38,18);not null" json:"fee"`
	OI      decimal.Decimal `gorm:"column:oi;type:decimal(38,18);not null" json:"oi"`
	Stake   decimal.Decimal `gorm:"column:stake;type:decimal(38,18);not null" json:"stake"`
	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*UserInfo) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*UserInfo) Indexes() []models.CustomIndex {
	return nil
}
