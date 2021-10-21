package main

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"os"
	"os/signal"
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
	db := database.GetDB()

	migrationAddUserInfoColumn(db, "AccTotalFee", logger)
	migrationAddUserInfoColumn(db, "InitTotalFee", logger)
	migrationAddSnapshotColumn(db, "AccTotalFee", logger)
	migrationAddSnapshotColumn(db, "InitTotalFee", logger)

	// backgroundCtx, stop := context.WithCancel(context.Background())
	// group, ctx := errgroup.WithContext(backgroundCtx)

	// fmt.Println(ctx)

	// client := graph.NewClient(logger, config.GetString("MAI3_TRADE_MINING_GRAPH_URL"))
	// users, err := client.GetUsersBasedOnBlockNumber(5573054)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// fmt.Println(len(users))
	// for _, u := range users {
	// 	// fmt.Println(u)
	// 	for _, m := range u.MarginAccounts {
	// 		if m.VaultFee.GreaterThan(decimal.Zero) || m.OperatorFee.GreaterThan(decimal.Zero) {
	// 			fmt.Printf("userID %s, totalFee %s, vaultFee %s, operatorFee %s\n", u.ID, m.TotalFee.String(), m.VaultFee.String(), m.OperatorFee.String())
	// 		}
	// 	}
	// }

	// go WaitExitSignal(stop, logger)

	// if err := group.Wait(); err != nil {
	// 	logger.Critical("service stopped: %s", err)
	// }
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

func migrationAddUserInfoColumn(db *gorm.DB, columnName string, logger logging.Logger) {
	isExist := db.Migrator().HasColumn(&mining.UserInfo{}, columnName)
	if isExist {
		logger.Info("column %s is exist in user_info table", columnName)
		return
	}
	err := db.Migrator().AddColumn(&mining.UserInfo{}, columnName)
	if err != nil {
		logger.Warn("failed to add column %s in user_info table", columnName)
	}
	return
}

func migrationAddSnapshotColumn(db *gorm.DB, columnName string, logger logging.Logger) {
	isExist := db.Migrator().HasColumn(&mining.Snapshot{}, columnName)
	if isExist {
		logger.Info("column %s is exist in snapshot table", columnName)
		return
	}
	err := db.Migrator().AddColumn(&mining.Snapshot{}, columnName)
	if err != nil {
		logger.Warn("failed to add column %s in snapshot table", columnName)
	}
	return
}
