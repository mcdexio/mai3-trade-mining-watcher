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

	mai3TradingMiningURL := config.GetString("MAI3_TRADE_MINING")

	feeSyncer, err := syncer.NewFeeSyncer(ctx, logger, mai3TradingMiningURL, 10)
	if err != nil {
		logger.Error("NewFeeSyncer fail:%s", err)
		os.Exit(-3)
	}
	group.Go(func() error {
		return feeSyncer.Run()
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
