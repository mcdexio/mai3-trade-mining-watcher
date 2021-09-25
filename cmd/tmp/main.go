package main

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"gorm.io/gorm/clause"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	name := "trading-mining"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)

	db := database.GetDB()

	db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&mining.Block{
		ID:     "0xec0644a24137ea17476bea10240bcd0386ae9699c4800543237afdf7d8344c17",
		Number: 1, // origin "1597480"
	})
	logger.Info("start")

	//backgroundCtx, stop := context.WithCancel(context.Background())
	//group, ctx := errgroup.WithContext(backgroundCtx)

	//arbBlockGraphUrl := config.GetString("ARB_BLOCKS_GRAPH_URL")

	//startTime, err := config.ParseTimeConfig(config.GetString("SYNCER_BLOCK_START_TIME"))
	//if err != nil {
	//	logger.Error("Failed to parse time config", err)
	//	return
	//} else {
	//	logger.Info("start time %s", startTime.String())
	//}
	//app := syncer.NewBlockSyncer(ctx, logger, arbBlockGraphUrl, &startTime)
	//go WaitExitSignal(stop, logger)
	//group.Go(func() error {
	//	return app.Run()
	//})

	//if err := group.Wait(); err != nil {
	//	logger.Critical("service stopped: %s", err)
	//}
}

func WaitExitSignal(ctxStop context.CancelFunc, logger logging.Logger) {
	var exitSignal = make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGTERM)
	signal.Notify(exitSignal, syscall.SIGINT)

	sig := <-exitSignal
	logger.Info("caught sig: %+v, Stopping...\n", sig)
	ctxStop()
}

func WaitExitSignalWithServer(ctxStop context.CancelFunc, logger logging.Logger, server *api.TMServer) {
	var exitSignal = make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGTERM)
	signal.Notify(exitSignal, syscall.SIGINT)

	sig := <-exitSignal
	logger.Info("caught sig: %+v, Stopping...\n", sig)
	if err := server.Shutdown(); err != nil {
		logger.Error("Server shutdown failed:%+v", err)
	}
	ctxStop()
}
