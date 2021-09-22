package miningdb

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"gorm.io/gorm"
	"strconv"
)

var logger = logging.NewLoggerTag("database")

// WatcherDBApp is the database application.
type WatcherDBApp struct {
}

// Models returns the models for a given database app.
func (e *WatcherDBApp) Models() []interface{} {
	return mining.AllModels
}

// IsEmpty check if a given database is empty.
func (e *WatcherDBApp) IsEmpty(db *gorm.DB) bool {
	return !db.Migrator().HasTable("mining")
}

// PreReset is executed before db is reset.
func (e *WatcherDBApp) PreReset(tx *gorm.DB) error {
	return nil
}

// PostReset is executed after db is reset.
func (e *WatcherDBApp) PostReset(tx *gorm.DB) error {
	return initSchemaVersion(tx)
}

func initSchemaVersion(db *gorm.DB) error {
	var result models.System
	err := db.Model(&models.System{}).Select("*").Where(
		"name = ?", "schema_version").Last(&result).Error
	var v int
	logger.Info("set default schema_version to 1")
	if err == nil {
		v, err = strconv.Atoi(result.Value)
		if err == nil {
			logger.Info("success to get last schema_version from system table")
		}
	}
	if res := db.Model(&models.System{}).Create(
		&models.System{
			Name:  types.SysVarSchemaVersion,
			Value: strconv.Itoa(v + 1),
		}); res.Error != nil {
		return res.Error
	}
	logger.Info("Initialized DB Schema version to %v.", v+1)
	return nil
}
