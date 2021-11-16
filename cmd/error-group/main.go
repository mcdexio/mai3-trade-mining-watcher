package main

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/block"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/mai3"
)

func main() {
	name := "error-group"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()
	logger := logging.NewLoggerTag(name)

	mai3GraphClients := make([]mai3.GraphInterface, 0)
	blockGraphClients := make([]block.BlockInterface, 0)
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
	mai3GraphClients = append(mai3GraphClients, arbMAI3GraphClient)
	// for arb block graph client
	arbBlockGraphClient := block.NewClient(logger, config.GetString("ARB_ONE_BLOCK_GRAPH_URL"))
	blockGraphClients = append(blockGraphClients, arbBlockGraphClient)

	timestamp := int64(1636959600)
	multiBlockGraphs := block.NewMultiClient(
		logger,
		blockGraphClients,
	)
	bns, err := multiBlockGraphs.GetMultiBlockNumberWithTS(timestamp)
	if err != nil {
		return
	}
	logger.Info("bns %+v", bns)

	multiMai3Graphs := mai3.NewMultiClient(
		logger,
		mai3GraphClients,
	)
	users, err := multiMai3Graphs.GetMultiUsersBasedOnMultiBlockNumbers(bns)
	if err != nil {
		return
	}
	for i, user := range users {
		if i == 0 {
			logger.Info("bsc length user %d", len(user))
		} else if i == 1 {
			logger.Info("arb length user %d", len(user))
		}
	}

	for i, bn := range bns {
		var graphClient mai3.GraphInterface
		if i == 0 {
			graphClient = bscMAI3GraphClient
		} else if i == 1 {
			graphClient = arbMAI3GraphClient
		}
		user, err := graphClient.GetUsersBasedOnBlockNumber(bn)
		if err != nil {
			return
		}
		if i == 0 {
			logger.Info("bsc length user %d", len(user))
		} else if i == 1 {
			logger.Info("arb length user %d", len(user))
		}
	}

	prices, err := multiMai3Graphs.GetMultiMarkPrices(bns)
	if err != nil {
		return
	}
	for perpID, price := range prices {
		logger.Info("perpID %s: prices %s", perpID, price.String())
	}
	for i, mai3GraphClient := range mai3GraphClients {
		prices, err = mai3GraphClient.GetMarkPrices(bns[i])
		if err != nil {
			return
		}
		for perpID, price := range prices {
			logger.Info("perpID %s: prices %s", perpID, price.String())
		}
	}
}
