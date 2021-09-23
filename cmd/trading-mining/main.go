package main

import (
	"context"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	cerrors "github.com/mcdexio/mai3-trade-mining-watcher/common/errors"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/syncer"
)

func main() {
	name := "trading-mining"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()

	logger := logging.NewLoggerTag(name)

	// Setup panic handler.
	cerrors.Initialize(logger)
	defer cerrors.Catch()

	logger.Info("%s service started.", name)
	logger.Info("Initializing.")

	backgroundCtx, stop := context.WithCancel(context.Background())
	go WaitExitSignal(stop, logger)
	group, ctx := errgroup.WithContext(backgroundCtx)

	intervalSec := config.GetInt("INTERVAL_SECOND", 60)

	syn, err := syncer.NewSyncer(ctx, logger, config.GetString("MAI3_TRADE_MINING"), config.GetString("MAI3_PERPETUAL"), "",intervalSec)
	if err != nil {
		logger.Error("syncer fail:%s", err)
		os.Exit(-3)
	}
	group.Go(func() error {
		return syn.Run()
	})

	// cal, err := trading_mining.NewCalculator(ctx, logger, intervalSec)
	// if err != nil {
	// 	logger.Error("calculator fail:%s", err)
	// 	os.Exit(-3)
	// }
	// group.Go(func() error {
	// 	return cal.Run()
	// })

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
