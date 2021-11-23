package main

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	go_ethereum "github.com/mcdexio/mai3-trade-mining-watcher/go-ethereum"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/block"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/mai3"
	"gorm.io/gorm"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/api"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"github.com/mcdexio/mai3-trade-mining-watcher/syncer"
	"golang.org/x/sync/errgroup"
)

func main() {
	name := "trading-mining"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)
	loggerGraph := logging.NewLoggerTag("graph")

	backgroundCtx, stop := context.WithCancel(context.Background())
	_, ctx := errgroup.WithContext(backgroundCtx)
	defer stop()

	mai3GraphClients := make([]mai3.GraphInterface, 0)
	blockGraphClients := make([]block.BlockInterface, 0)
	var mai3Graph *mai3.Client
	if env.BSCChainInclude() {
		// for bsc mai3 graph client
		bscBTCWhiteList := mai3.NewWhiteList(
			loggerGraph,
			config.GetString("BSC_BTC_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		bscETHWhiteList := mai3.NewWhiteList(
			loggerGraph,
			config.GetString("BSC_ETH_INVERSE_CONTRACT_WHITELIST0", ""),
			config.GetString("BSC_ETH_INVERSE_CONTRACT_WHITELIST1", ""),
		)
		bscSatsWhiteList := mai3.NewWhiteList(
			loggerGraph,
			config.GetString("BSC_SATS_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		bscMAI3GraphClient := mai3.NewClient(
			loggerGraph,
			config.GetString("BSC_MAI3_GRAPH_URL"),
			bscBTCWhiteList,
			bscETHWhiteList,
			bscSatsWhiteList,
			config.GetString("BSC_BTC_USD_PERP_ID", ""),
			config.GetString("BSC_ETH_USD_PERP_ID", ""),
		)
		mai3GraphClients = append(mai3GraphClients, bscMAI3GraphClient)

		goClient, err := go_ethereum.NewClient(logging.NewLoggerTag("go-eth"),
			config.GetString("BSC_PRC_SERVER"), ctx,
		)
		if err != nil {
			logger.Error("go-ethereum-client bsc err=%s", err)
			return
		}

		// for bsc block graph client
		bscBlockGraphClient := block.NewClient(loggerGraph, config.GetString("BSC_BLOCK_GRAPH_URL"), goClient)
		blockGraphClients = append(blockGraphClients, bscBlockGraphClient)
	}
	if env.ArbOneChainInclude() {
		// for arb mai3 graph client
		arbETHWhiteList := mai3.NewWhiteList(
			loggerGraph,
			config.GetString("ARB_ONE_ETH_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		arbMAI3GraphClient := mai3.NewClient(
			loggerGraph,
			config.GetString("ARB_ONE_MAI3_GRAPH_URL"),
			nil,
			arbETHWhiteList,
			nil,
			config.GetString("ARB_ONE_BTC_USD_PERP_ID", ""),
			config.GetString("ARB_ONE_ETH_USD_PERP_ID", ""),
		)
		mai3GraphClients = append(mai3GraphClients, arbMAI3GraphClient)
		mai3Graph = arbMAI3GraphClient

		goClient, err := go_ethereum.NewClient(logging.NewLoggerTag("go-eth"),
			config.GetString("ARB_ONE_PRC_SERVER"), ctx,
		)
		if err != nil {
			logger.Error("go-ethereum-client arb-one err=%s", err)
			return
		}

		// for arb block graph client
		arbBlockGraphClient := block.NewClient(loggerGraph, config.GetString("ARB_ONE_BLOCK_GRAPH_URL"), goClient)
		blockGraphClients = append(blockGraphClients, arbBlockGraphClient)
	}
	if env.ArbRinkebyChainInclude() {
		// for arb-rinkeby mai3 graph client
		arbRinkebyBTCWhiteList := mai3.NewWhiteList(
			loggerGraph,
			config.GetString("ARB_RINKEBY_BTC_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		arbRinkebyETHWhiteList := mai3.NewWhiteList(
			loggerGraph,
			config.GetString("ARB_RINKEBY_ETH_INVERSE_CONTRACT_WHITELIST0", ""),
		)
		arbRinkebyMAI3GraphClient := mai3.NewClient(
			loggerGraph,
			config.GetString("ARB_RINKEBY_MAI3_GRAPH_URL"),
			arbRinkebyBTCWhiteList,
			arbRinkebyETHWhiteList,
			nil,
			config.GetString("ARB_RINKEBY_BTC_USD_PERP_ID", ""),
			config.GetString("ARB_RINKEBY_ETH_USD_PERP_ID", ""),
		)
		mai3GraphClients = append(mai3GraphClients, arbRinkebyMAI3GraphClient)

		goClient, err := go_ethereum.NewClient(logging.NewLoggerTag("go-eth"),
			config.GetString("ARB_RINKEBY_PRC_SERVER"), ctx,
		)
		if err != nil {
			logger.Error("go-ethereum-client arb-one err=%s", err)
			return
		}

		// for arb-rinkeby block graph client
		arbRinkebyBlockGraphClient := block.NewClient(loggerGraph, config.GetString("ARB_RINKEBY_BLOCK_GRAPH_URL"), goClient)
		blockGraphClients = append(blockGraphClients, arbRinkebyBlockGraphClient)
	}

	multiMai3Graphs := mai3.NewMultiClient(
		loggerGraph,
		mai3GraphClients,
	)
	multiBlockGraphs := block.NewMultiClient(
		loggerGraph,
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

	multiBNs, multiUsers, multiPrices, err := syn.GetMultiChainInfo(1636934400)
	if err != nil {
		return
	}
	logger.Info("BNs %d", multiBNs[0])
	for _, u := range multiUsers[0] {
		var hasInverse bool
		for _, a := range u.MarginAccounts {
			if strings.HasPrefix(a.ID, "0xc7b2ad78fded2bbc74b50dc1881ce0f81a7a0cca-0") {
				hasInverse = true
				logger.Info("u.ID %s, totalFee %s, totalFeeFactor %s, vaultFee %s, vaultFeeFactor %s position %s",
					u.ID, a.TotalFee.String(), a.TotalFeeFactor.String(), a.VaultFee.String(), a.VaultFeeFactor.String(), a.Position)
			}
		}
		totalFee, daoFee, totalFeeFactor, daoFeeFactor := syncer.GetFeeValue(u.MarginAccounts)
		pv, err := syncer.GetOIValue(u.MarginAccounts, multiBNs[0], multiPrices, mai3Graph)
		if err != nil {
			return
		}
		if hasInverse {
			logger.Info("pv %s, totalFee %s, daoFee %s, totalFeeFactor %s, daoFeeFactor %s",
				pv.String(), totalFee.String(), daoFee.String(), totalFeeFactor.String(), daoFeeFactor.String())
		}
	}
	multiBNs, multiUsers, multiPrices, err = syn.GetMultiChainInfo(1637618400)
	if err != nil {
		return
	}
	logger.Info("BNs %d", multiBNs[0])
	for _, u := range multiUsers[0] {
		var hasInverse bool
		for _, a := range u.MarginAccounts {
			if strings.HasPrefix(a.ID, "0xc7b2ad78fded2bbc74b50dc1881ce0f81a7a0cca-0") {
				hasInverse = true
				logger.Info("u.ID %s, totalFee %s, totalFeeFactor %s, vaultFee %s, vaultFeeFactor %s position %s",
					u.ID, a.TotalFee.String(), a.TotalFeeFactor.String(), a.VaultFee.String(), a.VaultFeeFactor.String(), a.Position)
			}
		}
		totalFee, daoFee, totalFeeFactor, daoFeeFactor := syncer.GetFeeValue(u.MarginAccounts)
		pv, err := syncer.GetOIValue(u.MarginAccounts, multiBNs[0], multiPrices, mai3Graph)
		if err != nil {
			return
		}
		if hasInverse {
			logger.Info("pv %s, totalFee %s, daoFee %s, totalFeeFactor %s, daoFeeFactor %s",
				pv.String(), totalFee.String(), daoFee.String(), totalFeeFactor.String(), daoFeeFactor.String())
		}
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
