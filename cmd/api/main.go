package main

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/block"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/mai3"
	"gorm.io/gorm"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"github.com/mcdexio/mai3-trade-mining-watcher/validator"

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

	db := database.GetDB()
	migrationAddColumn(db, "AccTotalFee", logger)
	migrationAddColumn(db, "InitTotalFee", logger)
	migrationAddColumn(db, "Chain", logger)
	migrationAddColumn(db, "InitFeeFactor", logger)
	migrationAddColumn(db, "AccFeeFactor", logger)
	migrationAddColumn(db, "InitTotalFeeFactor", logger)
	migrationAddColumn(db, "AccTotalFeeFactor", logger)

	var AllModels = []interface{}{
		&mining.UserInfo{},
	}
	for _, model := range AllModels {
		err := database.CreateCustomIndices(db, model, "user_info")
		if err != nil {
			logger.Warn("err=%s", err)
			return
		}
		logger.Info("create new index for user_info")
	}

	AllModels = []interface{}{
		&mining.Snapshot{},
	}
	for _, model := range AllModels {
		err := database.CreateCustomIndices(db, model, "snapshot")
		if err != nil {
			logger.Warn("err=%s", err)
			return
		}
		logger.Info("create new index for snapshot")
	}

	backgroundCtx, stop := context.WithCancel(context.Background())
	group, ctx := errgroup.WithContext(backgroundCtx)

	tmServer := api.NewTMServer(ctx, logging.NewLoggerTag("server"))
	group.Go(func() error {
		return tmServer.Run()
	})

	internalServer := api.NewInternalServer(ctx, logging.NewLoggerTag("server"))
	group.Go(func() error {
		return internalServer.Run()
	})

	mai3GraphClients := make([]mai3.GraphInterface, 0)
	blockGraphClients := make([]block.BlockInterface, 0)
	if env.BSCChainInclude() {
		// for bsc mai3 graph client
		bscBTCWhiteList := mai3.NewWhiteList(
			logger,
			config.GetString("BSC_BTC_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		bscETHWhiteList := mai3.NewWhiteList(
			logger,
			config.GetString("BSC_ETH_INVERSE_CONTRACT_WHITELIST0", ""),
			config.GetString("BSC_ETH_INVERSE_CONTRACT_WHITELIST1", ""),
		)
		bscSatsWhiteList := mai3.NewWhiteList(
			logger,
			config.GetString("BSC_SATS_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		bscMAI3GraphClient := mai3.NewClient(
			logger,
			config.GetString("BSC_MAI3_GRAPH_URL"),
			bscBTCWhiteList,
			bscETHWhiteList,
			bscSatsWhiteList,
			config.GetString("BSC_BTC_USD_PERP_ID", ""),
			config.GetString("BSC_ETH_USD_PERP_ID", ""),
		)
		mai3GraphClients = append(mai3GraphClients, bscMAI3GraphClient)

		// for bsc block graph client
		bscBlockGraphClient := block.NewClient(logger, config.GetString("BSC_BLOCK_GRAPH_URL"))
		blockGraphClients = append(blockGraphClients, bscBlockGraphClient)
	}
	if env.ArbOneChainInclude() {
		// for arb mai3 graph client
		arbETHWhiteList := mai3.NewWhiteList(
			logger,
			config.GetString("ARB_ONE_ETH_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		arbRinkebyMAI3GraphClient := mai3.NewClient(
			logger,
			config.GetString("ARB_ONE_MAI3_GRAPH_URL"),
			nil,
			arbETHWhiteList,
			nil,
			config.GetString("ARB_ONE_BTC_USD_PERP_ID", ""),
			config.GetString("ARB_ONE_ETH_USD_PERP_ID", ""),
		)
		mai3GraphClients = append(mai3GraphClients, arbRinkebyMAI3GraphClient)

		// for arb block graph client
		arbBlockGraphClient := block.NewClient(logger, config.GetString("ARB_ONE_BLOCK_GRAPH_URL"))
		blockGraphClients = append(blockGraphClients, arbBlockGraphClient)
	}
	if env.ArbRinkebyChainInclude() {
		// for arb-rinkeby mai3 graph client
		arbRinkebyBTCWhiteList := mai3.NewWhiteList(
			logger,
			config.GetString("ARB_RINKEBY_BTC_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		arbRinkebyETHWhiteList := mai3.NewWhiteList(
			logger,
			config.GetString("ARB_RINKEBY_ETH_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		arbRinkebyMAI3GraphClient := mai3.NewClient(
			logger,
			config.GetString("ARB_RINKEBY_MAI3_GRAPH_URL"),
			arbRinkebyBTCWhiteList,
			arbRinkebyETHWhiteList,
			nil,
			config.GetString("ARB_RINKEBY_BTC_USD_PERP_ID", ""),
			config.GetString("ARB_RINKEBY_ETH_USD_PERP_ID", ""),
		)
		mai3GraphClients = append(mai3GraphClients, arbRinkebyMAI3GraphClient)

		// for arb-rinkeby block graph client
		arbRinkebyBlockGraphClient := block.NewClient(logger, config.GetString("ARB_RINKEBY_BLOCK_GRAPH_URL"))
		blockGraphClients = append(blockGraphClients, arbRinkebyBlockGraphClient)
	}

	multiMai3Graphs := mai3.NewMultiClient(
		logger,
		mai3GraphClients,
	)
	multiBlockGraphs := block.NewMultiClient(
		logger,
		blockGraphClients,
	)
	syn := syncer.NewSyncer(
		ctx,
		logger,
		multiMai3Graphs,
		multiBlockGraphs,
		config.GetInt64("DEFAULT_EPOCH_0_START_TIME"),
		config.GetInt64("SYNC_DELAY", 0),
		config.GetInt64("SNAPSHOT_INTERVAL", 3600),
	)
	go WaitExitSignalWithServer(stop, logger, tmServer, internalServer)

	vld, err := validator.NewValidator(
		&validator.Config{
			RoundInterval: mustParseDuration(config.GetString("VALIDATOR_ROUND_INTERVAL", "1m")),
			DatabaseURLs:  optional("DB_ARGS", "BACKUP_DB_ARGS"),
		},
		logging.NewLoggerTag("validator"),
	)
	if err != nil {
		logger.Warn("fail to start validate service, ignored: %s", err)
	} else {
		group.Go(func() error {
			return vld.Run(ctx)
		})
	}

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

func optional(names ...string) []string {
	var res []string
	for _, n := range names {
		s := config.GetString(n, "__NO_VALUE__")
		if s != "__NO_VALUE__" {
			res = append(res, s)
		}
	}
	return res
}

func mustParseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
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
	logger.Info("migration: add new column %s in user_info table", columnName)
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
	logger.Info("migration: add new column %s in snapshot table", columnName)
	return
}

func migrationAddColumn(db *gorm.DB, columnName string, logger logging.Logger) {
	migrationAddUserInfoColumn(db, columnName, logger)
	migrationAddSnapshotColumn(db, columnName, logger)
}
