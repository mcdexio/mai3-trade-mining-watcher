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

	syn := syncer.NewSyncer(
		ctx,
		logger,
		config.GetString("MAI3_TRADE_MINING_GRAPH_URL"),
		config.GetString("ARB_BLOCKS_GRAPH_URL"),
	)

	priceMap, err := syn.GetMarkPrices(4933593)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(priceMap)

	blockNumber, err := syn.TimestampToBlockNumber(1632674652)
	if err != nil {
		fmt.Println(err)
		return
	}
	user, err := syn.GetUsersBasedOnBlockNumber(blockNumber)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(user))

	price, err := syn.GetMarkPriceBasedOnBlockNumber(2771249, "0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13", 0)
	if err != nil {
		return
	}
	fmt.Println(price.String())

	// poolAddr, userId, perpetualIndex, err := syn.GetPoolAddrIndexUserID("0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0-0x00233150044aec4cba478d0bf0ecda0baaf5ad19")
	// if err != nil {
	// 	return
	// }
	// fmt.Println(poolAddr)
	// fmt.Println(userId)
	// fmt.Println(perpetualIndex)

	go WaitExitSignal(stop, logger)
	syn.Init()
	group.Go(func() error {
		return syn.Run()
	})

	if err := group.Wait(); err != nil {
		logger.Critical("service stopped: %s", err)
	}
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
