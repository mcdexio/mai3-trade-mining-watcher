package mining

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
)

type Progress struct {
	TableName  types.TableName `gorm:"column:table_name;type:varchar(129);not null;primary_key" json:"table_name"`
	From       int64           `gorm:"column:from;type:bigint;not null" json:"from"`
	To         int64           `gorm:"column:to;type:bigint;not null" json:"to"`
	Checkpoint int64           `gorm:"column:checkpoint;type:bigint;not null" json:"checkpoint"`

	models.Base
}
