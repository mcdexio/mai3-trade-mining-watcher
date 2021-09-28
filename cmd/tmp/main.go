package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"golang.org/x/sync/errgroup"
)

func main() {
	name := "trading-mining"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)

	backgroundCtx, stop := context.WithCancel(context.Background())
	group, ctx := errgroup.WithContext(backgroundCtx)

	tmServer := api.NewTMServer(ctx, logger)
	group.Go(func() error {
		return tmServer.Run()
	})

	inServer := api.NewInternalServer(ctx, logger)
	group.Go(func() error {
		return inServer.Run()
	})

	go WaitExitSignalWithServer(stop, logger, tmServer, inServer)

	if err := group.Wait(); err != nil {
		logger.Critical("service stopped: %s", err)
	}
}

func WaitExitSignalWithServer(ctxStop context.CancelFunc, logger logging.Logger, server *api.TMServer, inServer *api.InternalServer) {
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
