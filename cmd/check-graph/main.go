package main

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/block"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/mai3"
)

func main() {
	timestamp := int64(1636959600)
	name := "check-graph"
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)

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
	bscBlockGraphClient := block.NewClient(logger, config.GetString("BSC_BLOCK_GRAPH_URL"), nil)

	lastBscBN, lastBscTS, err := bscBlockGraphClient.GetLatestBlockNumberAndTS()
	if err != nil {
		logger.Error("fail to get bsc latest bn err=%s", err)
	} else {
		logger.Info("bsc last bn %d ts %d", lastBscBN, lastBscTS)
	}
	bscBN, err := bscBlockGraphClient.GetBlockNumberWithTS(timestamp)
	if err != nil {
		logger.Error("fail to get bsc bn from ts %d", timestamp)
	} else {
		logger.Info("bscBN %d", bscBN)
	}
	bscUsers, err := bscMAI3GraphClient.GetUsersBasedOnBlockNumber(bscBN)
	if err != nil {
		logger.Error("fail to get bsc users bn %d", bscBN)
	}
	logger.Info("length bsc users %d", len(bscUsers))

	arbETHWhiteList := mai3.NewWhiteList(
		logger,
		config.GetString("ARB_ONE_ETH_INVERSE_CONTRACT_WHITELIST0", ""),
	)
	arbMAI3GraphClient := mai3.NewClient(
		logger,
		config.GetString("ARB_ONE_MAI3_GRAPH_URL"),
		nil,
		arbETHWhiteList,
		nil,
		config.GetString("ARB_ONE_BTC_USD_PERP_ID", ""),
		config.GetString("ARB_ONE_ETH_USD_PERP_ID", ""),
	)

	// for arb block graph client
	arbBlockGraphClient := block.NewClient(logger, config.GetString("ARB_ONE_BLOCK_GRAPH_URL"), nil)

	lastArbBN, lastArbTS, err := arbBlockGraphClient.GetLatestBlockNumberAndTS()
	if err != nil {
		logger.Error("fail to get arb latest bn err=%s", err)
	}
	logger.Info("arb last bn %d ts %d", lastArbBN, lastArbTS)
	arbBN, err := arbBlockGraphClient.GetBlockNumberWithTS(timestamp)
	if err != nil {
		logger.Error("fail to get arb bn from ts %d", timestamp)
	} else {
		logger.Info("arbBN %d", arbBN)
	}

	arbUsers, err := arbMAI3GraphClient.GetUsersBasedOnBlockNumber(arbBN)
	if err != nil {
		logger.Error("fail to get arb users bn %d", arbBN)
	} else {
		logger.Info("length arb users %d", len(arbUsers))
	}
}
