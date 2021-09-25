package mining

import "github.com/mcdexio/mai3-trade-mining-watcher/database/models"

type Block struct {
	ID        string `gorm:"column:id;type:varchar(129);not null;primary_key" json:"id"`
	Number    int64  `gorm:"column:number;type:bigint;not null" json:"number"`
	Timestamp int64  `gorm:"column:timestamp;type:bigint;not null" json:"timestamp"`
	models.Base
}

// ForeignKeyConstraints create foreign key constraints.
func (*Block) ForeignKeyConstraints() []models.ForeignKeyConstraint {
	return nil
}

// Indexes returns information to create index.
func (*Block) Indexes() []models.CustomIndex {
	return nil
}
