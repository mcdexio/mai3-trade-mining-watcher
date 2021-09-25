package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

type MarkPrice struct {
	ID        int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	PoolAddr string          `gorm:"column:pool_addr;type:varchar(128);not null" json:"pool_addr"`
	Price     decimal.Decimal `gorm:"column:price;type:decimal(38,18);not null" json:"price"`
	PerpetualIndex int64 `gorm:"column:perpetual_index;type:bigint;not null" json:"perpetual_index"`


	models.Base
}
// ForeignKeyConstraints create foreign key constraints.
func (*MarkPrice) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*MarkPrice) Indexes() []models.CustomIndex {
	return nil
}
