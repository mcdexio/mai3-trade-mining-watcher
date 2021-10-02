package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexflint/go-arg"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/validator"
)

func main() {
	name := "trading-mining-validator"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)

	// postgres://mcdex@localhost:5432/mcdex?sslmode=disable
	args := new(validator.Config)
	arg.Parse(args)
	logger.Info("using config %+v", args)

	v, err := validator.NewValidator(args, logger)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		v.Run(ctx)
	}()
	wait(cancelFunc)
}

func wait(stop context.CancelFunc) {
	var exitSignal = make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGTERM)
	signal.Notify(exitSignal, syscall.SIGINT)
	<-exitSignal
	stop()
}
