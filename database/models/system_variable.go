package models

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
)

// System defines the table to store system variables.
type System struct {
	Base

	ID    int64        `gorm:"column:id;primary_key;AUTO_INCREMENT;not null" json:"id"`
	Name  types.SysVar `gorm:"column:name;primary_key;type:varchar(50);not null" json:"-" bigquery:"primary_key"`
	Value string       `gorm:"column:value;primary_key;type:varchar(512)" json:"-"`
}

// Purged implements bigquery.Exportable interface.
func (s System) Purged() interface{} {
	return s
}
