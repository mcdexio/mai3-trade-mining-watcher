package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/shopspring/decimal"
)

// Mining defines struct to contain information of a user
type Mining struct {
	models.Base

	ID       int64           `gorm:"column:id;primary_key;AUTO_INCREMENT;not null"`
	User     string          `gorm:"column:user;type:varchar(128);not null" json:"user"`
	Position decimal.Decimal `gorm:"column:position;type:decimal(38,18);" json:"position"`

	// Timestamps.
	Timestamp int64 `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`

	Perpetual Perpetual `gorm:"ForeignKey:PerpetualID;AssociationForeignKey:ID"`
}

// ForeignKeyConstraints create foreign key constraints.
func (*Mining) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return []models.ForeignKeyConstraint{
		{
			Field:    "perpetual_id",
			Dest:     "\"perpetual\"(id)",
			OnDelete: "RESTRICT",
			OnUpdate: "RESTRICT",
		},
	}
}

// Indexes returns information to create index.
func (*Mining) Indexes() []models.CustomIndex {
	return nil
}
