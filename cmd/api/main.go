package main

import (
	"context"
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

	backgroundCtx, stop := context.WithCancel(context.Background())
	group, ctx := errgroup.WithContext(backgroundCtx)

	startTimeString := config.GetString("START_TIME")
	startTime, err := config.ParseTimeConfig(startTimeString)
	if err != nil {
		logger.Error("Failed to parse start time:%s", err)
		return
	}
	intervalSec := config.GetInt("INTERVAL_SECOND")

	syn, err := syncer.NewSyncer(
		ctx,
		logger,
		config.GetString("MAI3_TRADE_MINING_GRAPH_URL"),
		intervalSec,
		&startTime,
	)
	if err != nil {
		logger.Error("Failed to start syncer:%s", err)
		os.Exit(-3)
	}

	tmServer, err := api.NewTMServer(ctx, logger, 120)
	if err != nil {
		logger.Error("Failed to trading mining server:%s", err)
		os.Exit(-3)
	}

	go WaitExitSignal(stop, logger, tmServer)
	group.Go(func() error {
		return syn.Run()
	})
	group.Go(func() error {
		return tmServer.Run()
	})

	if err := group.Wait(); err != nil {
		logger.Critical("service stopped: %s", err)
	}
}

func WaitExitSignal(ctxStop context.CancelFunc, logger logging.Logger, server *api.TMServer) {
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
