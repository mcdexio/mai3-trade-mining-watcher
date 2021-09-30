package syncer

import (
	"math"

	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
)

type ScoreCalculator struct {
}

func (sc ScoreCalculator) calcStakeScore(user User, timestamp int64) decimal.Decimal {
	// already unlocked core == 0
	if user.UnlockMCBTime < timestamp {
		return decimal.Zero
	}
	// floor to 1 if less than 1 day
	days := int64(math.Ceil(float64(user.UnlockMCBTime-timestamp) / 86400))
	return decimal.NewFromInt(days).Mul(user.StakedMCB)
}

func (sc ScoreCalculator) calcScore(epoch *mining.Schedule, ui *mining.UserInfo, timeElapsed int64) decimal.Decimal {
	// early exit
	if ui.AccFee.IsZero() {
		return decimal.Zero
	}
	fee := ui.AccFee.Sub(ui.InitFee)
	if fee.IsZero() {
		return decimal.Zero
	}
	stake := ui.AccStakeScore.Add(ui.CurStakeScore)
	if stake.IsZero() {
		return decimal.Zero
	}
	posVal := ui.AccPosValue.Add(ui.CurPosValue)
	if posVal.IsZero() {
		return decimal.Zero
	}
	// decimal package has issue on pow function
	score := math.Pow(unwrap(fee), unwrap(epoch.WeightFee)) *
		math.Pow(unwrap(stake)/float64(timeElapsed), unwrap(epoch.WeightMCB)) *
		math.Pow(unwrap(posVal)/float64(timeElapsed), unwrap(epoch.WeightOI))
	return decimal.NewFromFloat(score)
}

func unwrap(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}
