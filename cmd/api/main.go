package main

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/syncer"
	"golang.org/x/sync/errgroup"
)

func main() {
	name := "trading-mining"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)

	database.Initialize()
	if env.ResetDatabase() {
		database.Reset(database.GetDB(), types.Watcher, true)
	}

	backgroundCtx, stop := context.WithCancel(context.Background())
	group, ctx := errgroup.WithContext(backgroundCtx)

	tmServer := api.NewTMServer(ctx, logger)
	group.Go(func() error {
		return tmServer.Run()
	})

	internalServer := api.NewInternalServer(ctx, logger)
	group.Go(func() error {
		return internalServer.Run()
	})

	syn := syncer.NewSyncer(
		ctx,
		logger,
		config.GetString("MAI3_TRADE_MINING_GRAPH_URL"),
		config.GetString("ARB_BLOCKS_GRAPH_URL"),
		config.GetInt64("DEFAULT_EPOCH_0_START_TIME"),
	)
	go WaitExitSignalWithServer(stop, logger, tmServer, internalServer)
	group.Go(func() error {
		return syn.Run()
	})

	if err := group.Wait(); err != nil {
		logger.Critical("service stopped: %s", err)
	}
}

func WaitExitSignalWithServer(
	ctxStop context.CancelFunc, logger logging.Logger, server *api.TMServer,
	inServer *api.InternalServer) {
	var exitSignal = make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGTERM)
	signal.Notify(exitSignal, syscall.SIGINT)

	sig := <-exitSignal
	logger.Info("caught sig: %+v, Stopping...\n", sig)
	if err := server.Shutdown(); err != nil {
		logger.Error("Server shutdown failed:%+v", err)
	}
	if err := inServer.Shutdown(); err != nil {
		logger.Error("Server shutdown failed:%+v", err)
	}
	ctxStop()
}
