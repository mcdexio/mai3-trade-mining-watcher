package main

import (
	"context"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type epochStats struct {
	epoch         int64
	totalTrader   int64
	totalFee      decimal.Decimal
	totalMCBScore decimal.Decimal
	totalOI       decimal.Decimal
	totalScore    decimal.Decimal
}

func main() {
	name := "trading-mining"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)
	// db := database.GetDB()

	score := make(map[int]epochStats)
	fmt.Println(score[0].totalScore)
	fmt.Println(score[1])

	backgroundCtx, stop := context.WithCancel(context.Background())
	group, ctx := errgroup.WithContext(backgroundCtx)

	fmt.Println(ctx)

	client := graph.NewMAI3Client(logger, config.GetString("MAI3_TRADE_MINING_GRAPH_URL"))
	users, err := client.GetUsersBasedOnBlockNumber(2771249)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(users))
	count := 0
	for _, u := range users {
		// fmt.Println(u)
		for _, m := range u.MarginAccounts {
			perpId := strings.Join(strings.Split(m.ID, "-")[:2], "-")

			if perpId == "0xf6b2d76c248af20009188139660a516e5c4e0532-1" {
				count += 1
				fmt.Println(u.ID)
			}
		}
	}
	fmt.Println(count)
	prices, err := client.GetMarkPrices(2771249)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(prices)

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
