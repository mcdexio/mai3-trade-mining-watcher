package main

import (
	"context"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
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

	b, err := env.InInverseContractWhiteList("0x3d3744dc7a17d757a2568ddb171d162a7e12f80-0")
	fmt.Println(err)
	fmt.Println(b)

	b, err = env.InInverseContractWhiteList("0x727e5a9a04080741cbc8a2dc891e28ca8af6537e-0")
	fmt.Println(err)
	fmt.Println(b)

	b, err = env.InInverseContractWhiteList("0x727e5a9a04080741cbc8a2dc891e28ca8af6537e-1")
	fmt.Println(err)
	fmt.Println(b)

	backgroundCtx, stop := context.WithCancel(context.Background())
	group, ctx := errgroup.WithContext(backgroundCtx)

	fmt.Println(ctx)

	go WaitExitSignal(stop, logger)

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
