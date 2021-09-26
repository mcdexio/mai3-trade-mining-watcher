package main

import (
	"context"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/syncer"
	"golang.org/x/sync/errgroup"
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
	// db := database.GetDB()

	backgroundCtx, stop := context.WithCancel(context.Background())
	group, ctx := errgroup.WithContext(backgroundCtx)

	syncerBlockStartTime := config.GetString("SYNCER_BLOCK_START_TIME")
	startTime, err := config.ParseTimeConfig(syncerBlockStartTime)
	if err != nil {
		logger.Error("Failed to parse block start time %s:%s", err)
		os.Exit(-3)
		return
	}

	syn := syncer.NewSyncer(
		ctx,
		logger,
		config.GetString("MAI3_TRADE_MINING_GRAPH_URL"),
		config.GetString("ARB_BLOCKS_GRAPH_URL"),
		&startTime,
	)

	// blockNumber, err := syn.TimestampToBlockNumber(1632585600)
	// if err != nil {
	// 	return
	// }
	// user, err := syn.GetUsersBasedOnBlockNumber(blockNumber)
	// if err != nil {
	// 	return
	// }
	// fmt.Println(len(user))

	price, err := syn.GetMarkPriceBasedOnBlockNumber(2771249, "0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13", 0)
	if err != nil {
		return
	}
	fmt.Println(price.String())

	go WaitExitSignal(stop, logger)
	syn.Init()
	group.Go(func() error {
		return syn.Run()
	})

	if err := group.Wait(); err != nil {
		logger.Critical("service stopped: %s", err)
	}

	return
	// var blockInfo mining.Block
	// err := db.Model(&mining.Block{}).Limit(1).Order(
	// 	"number desc").Where(
	// 	"timestamp < ?", 1632499200,
	// ).Scan(&blockInfo).Error
	// if err != nil {
	// 	logger.Error("Failed to get schedule %s", err)
	// 	return
	// }
	// fmt.Println(blockInfo)
	// now := time.Now().Unix()
	// for _, schedule := range schedules {
	// 	if now > schedule.StartTime && now < schedule.EndTime {
	// 		epoch := schedule.Epoch
	// 		fmt.Println(epoch)
	// 		break
	// 	}
	// }

	// server := api.NewTMServer(ctx, logger, 60)
	// go WaitExitSignalWithServer(stop, logger, server)
	// group.Go(func() error {
	// 	return server.Run()
	// })

	// arbBlockGraphUrl := config.GetString("ARB_BLOCKS_GRAPH_URL")
	// startTime, err := config.ParseTimeConfig(config.GetString("SYNCER_BLOCK_START_TIME"))
	// if err != nil {
	// 	logger.Error("Failed to parse time config", err)
	// 	return
	// } else {
	// 	logger.Info("start time %s", startTime.String())
	// }
	// app := syncer.NewBlockSyncer(ctx, logger, arbBlockGraphUrl, &startTime)
	// group.Go(func() error {
	// 	return app.Run()
	// })

	// mai3GraphUrl := config.GetString("MAI3_TRADE_MINING_GRAPH_URL")
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
