package main

import (
	"fmt"
	cerrors "github.com/mcdexio/mai3-trade-mining-watcher/common/errors"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
)

func main() {
	name := "tmp"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()

	logger := logging.NewLoggerTag(name)

	// Setup panic handler.
	cerrors.Initialize(logger)
	defer cerrors.Catch()

	logger.Info("%s service started.", name)
	logger.Info("Initializing.")

	db := database.GetDB()
	var result models.System
	err := db.Model(&models.System{}).Select("*").Where(
		"name = ?", "schema_version").Last(&result).Error
	if err != nil {
		panic(fmt.Errorf("failed to get value from system table err=%w", err))
	}
	logger.Info("%+v", result)
}
