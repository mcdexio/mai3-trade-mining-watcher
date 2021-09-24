package main

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/syncer"
	trading_mining "github.com/mcdexio/mai3-trade-mining-watcher/trading-mining"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ParseTimeConfig parses config to time with format.
func ParseTimeConfig(t string) (time.Time, error) {
	return time.Parse(time.RFC3339, t)
}

func main() {
	name := "trading-mining"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)

	backgroundCtx, stop := context.WithCancel(context.Background())
	go WaitExitSignal(stop, logger)
	group, ctx := errgroup.WithContext(backgroundCtx)

	startTimeString := config.GetString("START_TIME")
	startTime, err := ParseTimeConfig(startTimeString)
	if err != nil {
		logger.Error("Failed to parse start date %s", )
		return
	}
	intervalSec := config.GetInt("INTERVAL_SECOND")

	syn, err := syncer.NewSyncer(
		ctx,
		logger,
		config.GetString("MAI3_TRADE_MINING_URL"),
		config.GetString("MAI3_PERPETUAL_URL"),
		config.GetString("MAI3_STAKE_URL", ""),
		intervalSec,
		&startTime,
	)
	if err != nil {
		logger.Error("syncer fail:%s", err)
		os.Exit(-3)
	}
	group.Go(func() error {
		return syn.Run()
	})

	cal, err := trading_mining.NewCalculator(ctx, logger, intervalSec, &startTime)
	if err != nil {
		logger.Error("calculator fail:%s", err)
		os.Exit(-3)
	}
	group.Go(func() error {
		return cal.Run()
	})

	tmServer, err := api.NewTMServer(ctx, logger, 120)
	if err != nil {
		logger.Error("trading mining server failed:%s", err)
	}
	group.Go(func() error {
		return tmServer.Run()
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
