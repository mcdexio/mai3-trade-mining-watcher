package main

import (
	cerrors "github.com/mcdexio/mai3-trade-mining-watcher/common/errors"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/shopspring/decimal"
)

type info struct {
	Fee        decimal.Decimal
	Stack      decimal.Decimal
	EntryValue decimal.Decimal
	Timestamp  int64
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

	// db := database.GetDB()
	// var feeResults []struct {
	// 	UserAdd   string
	// 	Fee       decimal.Decimal
	// 	Timestamp int64
	// }
	// var stackResults []struct {
	// 	UserAdd string
	// 	Stack   decimal.Decimal
	// }
	// var positionResults []struct {
	// 	UserAdd    string
	// 	EntryValue decimal.Decimal
	// }
	// var userInfoResults []struct {
	// 	UserAdd string
	// 	Fee     decimal.Decimal
	// 	Stack   decimal.Decimal
	// 	OI      decimal.Decimal
	// 	Timestamp int64
	// }

	// now := time.Now()
	// fmt.Println(now.Minute())
	// fmt.Println(now.Unix())

	// duration, _ := time.ParseDuration("-8h")
	// then := now.Add(duration)
	// lastTimestamp := &then

	// err := db.Model(&mining.Fee{}).Select("DISTINCT user_add ").Where("created_at > ?", lastTimestamp).Scan(&feeResults).Error
	// if err != nil {
	// 	logger.Error("failed to get fee %s", err)
	// 	return
	// }
	// count := len(feeResults)
	// err = db.Model(&mining.Fee{}).Select("user_add, fee, timestamp").Where("created_at > ?", lastTimestamp).Scan(&feeResults).Error
	// if err != nil {
	// 	logger.Error("failed to get fee %s", err)
	// 	return
	// }
	// sort.Slice(feeResults, func(i, j int) bool { return feeResults[i].Timestamp < feeResults[j].Timestamp })
	// if err != nil {
	// 	logger.Error("failed to get fee %s", err)
	// 	return
	// }
	// fmt.Println(len(feeResults))
	// uniqueUser := make(map[string]*info)

	//for i, r := range feeResults {
	//	if i == count-1 {
	//		// we only get the latest fee
	//		break
	//	}
	//	uniqueUser[r.UserAdd] = &info{Fee: r.Fee, Timestamp: r.Timestamp}

	//	err = db.Model(&mining.Stack{}).Select("user_add, AVG(stack) as stack").Where("user_add = ? and created_at > ?", r.UserAdd, lastTimestamp).Group("user_add").Scan(&stackResults).Error
	//	if err != nil {
	//		logger.Error("failed to get stack %s", err)
	//		return
	//	}

	//	err = db.Model(&mining.Position{}).Select("user_add, AVG(entry_value) as entry_value").Where("user_add = ? and created_at > ?", r.UserAdd, lastTimestamp).Group("user_add").Scan(&positionResults).Error
	//	if len(positionResults) == 0 {
	//		fmt.Println(r.UserAdd)
	//		break
	//	} else {
	//		fmt.Println(positionResults)
	//	}
	//}

	// for _, r := range feeResults {
	// 	fmt.Println(r)
	// }
	// uniqueUser := make(map[string]bool)
	// for i, r := range feeResults {
	// 	fmt.Println(r.UserAdd)
	// 	err = db.Model(&mining.UserInfo{}).Select("fee").Where("user_add = ?", r.UserAdd).Scan(&userInfoResults).Error
	// 	fmt.Println(len(userInfoResults))

	// 	if err != nil {
	// 		logger.Error("failed to get user info %s", err)
	// 	}

	// 	uniqueUser[r.UserAdd] = true
	// 	if i == 1 {
	// 		break
	// 	}
	// }

}
