package main

import (
	"fmt"
	cerrors "github.com/mcdexio/mai3-trade-mining-watcher/common/errors"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"math"
)

type EpochTradingMiningResp struct {
	Trader     string `json:"trader"`
	Fee        string `json:"fee"`
	OI         string `json:"oi"`
	Stake      string `json:"stake"`
	Score      string `json:"score"`
	Epoch      int    `json:"epoch"`
	Proportion string `json:"proportion"`
	// Timestamps.
	Timestamp int64 `json:"timestamp"`
}

type QueryTradingMiningResp struct {
	Epochs map[int]*EpochTradingMiningResp `json:"epoch"`
}

func main() {
	name := "tmp"
	// Initialize logger.
	logging.Initialize(name)
	defer logging.Finalize()

	logger := logging.NewLoggerTag(name)

	// Setup panic handler.
	cerrors.Initialize(logger)
	defer cerrors.Catch()

	logger.Info("%s service started.", name)
	logger.Info("Initializing.")

	// response := QueryTradingMiningResp{
	// 	Epochs: make(map[int]*EpochTradingMiningResp, 0),
	// }
	var epochs []struct {
		Epoch int
	}
	db := database.GetDB()
	err := db.Model(&mining.UserInfo{}).Limit(1).Order("epoch desc").Select("epoch").Scan(&epochs).Error
	if err != nil {
		logger.Error("failed to get user info %s", err)
	}
	fmt.Println(epochs)

	// var result mining.UserInfo
	// err := db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee, oi, stake, timestamp").Where(
	// 	"trader = ?", "0xe2163420248c37d2d16378e0761c95646659762b").Scan(&result).Error
	// if err != nil {
	// 	panic(fmt.Errorf("failed to get value from system table err=%w", err))
	// }
	// logger.Info("%+v", result)
}

func calScore(fee, oi, stake decimal.Decimal) decimal.Decimal {
	feeInflate, _ := fee.Float64()
	oiInflate, _ := oi.Float64()
	stakeInflate, _ := stake.Float64()
	score := math.Pow(feeInflate, 0.7) + math.Pow(oiInflate, 0.3) + math.Pow(stakeInflate, 0.3)
	return decimal.NewFromFloat(score)
}
