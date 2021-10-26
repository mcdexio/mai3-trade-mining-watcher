package syncer

import (
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"math"
	"strconv"
	"strings"
)

func norm(ts int64) int64 {
	return ts - ts%60
}

func normN(ts int64, inv int64) int64 {
	return ts - ts%inv
}

func splitMarginAccountID(marginAccountID string) (poolAddr, userId string, perpIndex int, err error) {
	rest := strings.Split(marginAccountID, "-")
	perpIndex, err = strconv.Atoi(rest[1])
	if err != nil {
		err = fmt.Errorf("fail to spliMarginAccountID from id=%s err=%s", marginAccountID, err)
		return
	}
	poolAddr = rest[0]
	userId = rest[2]
	return
}

func getEstimatedStakeScore(
	nowTimestamp int64, epoch *mining.Schedule, unlockTime int64,
	currentStakingReward decimal.Decimal,
) decimal.Decimal {
	// fmt.Printf("nowTS %d, epochEndTime %d, unlockTime %d\n", nowTimestamp, epoch.EndTime, unlockTime)
	// A = (1 - Floor(RemainEpochSeconds / 86400) / UnlockTimeInDays / 2) * CurrentStakingReward * RemainEpochMinutes
	// EstimatedAverageStakingScore  = (CumulativeStakingScore + A) / TotalEpochMinutes

	// floor to 0 if less than 1 day
	endTimeMinusNowTS := float64(epoch.EndTime - nowTimestamp)
	if endTimeMinusNowTS <= 0 {
		// there is no remainTime in this epoch
		return decimal.Zero
	}
	remainEpochDays := math.Floor(endTimeMinusNowTS / 86400)
	// fmt.Printf("remainEpochDays %v\n", remainEpochDays)
	// ceil to 1 if less than 1 day
	unlockTimeInDays := math.Ceil(float64(unlockTime-nowTimestamp) / 86400)
	// fmt.Printf("unlockTimeInDays %v\n", unlockTimeInDays)
	var remainProportion decimal.Decimal
	if unlockTimeInDays <= 0 {
		// there is no stake time
		return decimal.Zero
	} else {
		remainProportion = decimal.NewFromFloat(1.0 - (remainEpochDays / unlockTimeInDays / 2.0))
	}
	// fmt.Printf("remainProportion %v\n", remainProportion)
	// ceil to 1 if less than 1 minute
	remainEpochMinutes := decimal.NewFromFloat(math.Ceil(endTimeMinusNowTS / 60))
	// fmt.Printf("remainEpochMinutes %v\n", remainEpochMinutes)
	estimatedSS := remainProportion.Mul(currentStakingReward).Mul(remainEpochMinutes)
	// fmt.Printf("estimatedStakeScore %v\n", estimatedSS)
	return estimatedSS
}

func getStakeScore(curTime int64, unlockTime int64, staked decimal.Decimal) decimal.Decimal {
	// ss is (unlock time - now) * u.StackedMCB <=> s = n * t
	if unlockTime < curTime {
		return decimal.Zero
	}
	// floor to 1 if less than 1 day
	days := int64(math.Ceil(float64(unlockTime-curTime) / 86400))
	return decimal.NewFromInt(days).Mul(staked)
}

func getScore(epoch *mining.Schedule, ui *mining.UserInfo, remains decimal.Decimal) decimal.Decimal {
	if ui.AccTotalFee.IsZero() {
		return decimal.Zero
	}
	fee := decimal.Zero
	if epoch.Epoch == 0 {
		// epoch 0 is totalFee
		fee = ui.AccTotalFee.Sub(ui.InitTotalFee)
	} else {
		fee = ui.AccFee.Sub(ui.InitFee)
	}
	if fee.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	stake := ui.AccStakeScore.Add(ui.CurStakeScore)
	stake = stake.Add(ui.EstimatedStakeScore)
	if stake.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	// EstimatedOpenInterest = (CumulativeOpenInterest + CurrentOpenInterest * RemainEpochMinutes) / TotalEpochMinutes
	posVal := ui.AccPosValue.Add(ui.CurPosValue.Mul(remains))
	if posVal.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	// ceil to 1 if less than 1 minute
	totalEpochMinutes := math.Ceil(float64(epoch.EndTime-epoch.StartTime) / 60)

	// decimal package has issue on pow function
	wFee, _ := epoch.WeightFee.Float64()
	wStake, _ := epoch.WeightMCB.Float64()
	wPos, _ := epoch.WeightOI.Float64()
	feeFloat, _ := fee.Float64()
	stakeFloat, _ := stake.Float64()
	posValFloat, _ := posVal.Float64()
	score := math.Pow(feeFloat, wFee) * math.Pow(stakeFloat/totalEpochMinutes, wStake) * math.Pow(
		posValFloat/totalEpochMinutes, wPos)
	if math.IsNaN(score) {
		return decimal.Zero
	}
	return decimal.NewFromFloat(score)
}

func GetRemainMinutes(timestamp int64, epoch *mining.Schedule) decimal.Decimal {
	minuteCeil := int64(math.Floor((float64(timestamp) - float64(epoch.StartTime)) / 60.0))
	remains    := decimal.NewFromInt((epoch.EndTime-epoch.StartTime)/60.0 - minuteCeil) // total epoch in minutes
	return remains
}
