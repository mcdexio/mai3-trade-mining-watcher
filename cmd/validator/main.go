package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexflint/go-arg"
	"github.com/mcdexio/mai3-trade-mining-watcher/validator"
)

func main() {
	// postgres://mcdex@localhost:5432/mcdex?sslmode=disable
	args := new(validator.Config)
	arg.Parse(args)
	_, err := validator.NewValidator(args)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("connected")
	_, cancelFunc := context.WithCancel(context.Background())
	wait(cancelFunc)
}

func wait(stop context.CancelFunc) {
	var exitSignal = make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGTERM)
	signal.Notify(exitSignal, syscall.SIGINT)
	<-exitSignal
	stop()
}
