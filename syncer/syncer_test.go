package syncer

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
)

func TestSyncer_getScore(t *testing.T) {

	fee := decimal.NewFromFloat(3.498035787702170790).Sub(decimal.Zero)
	stake := decimal.NewFromFloat(0.0).Add(decimal.NewFromFloat(0.0))
	posVal := decimal.NewFromFloat(0.0).Add(decimal.NewFromFloat(0.0))
	t.Log("1!",
		fee.Pow(decimal.NewFromFloat(0.3)).
			Mul(stake.Pow(decimal.NewFromFloat(0.3))).
			Mul(posVal.Pow(decimal.NewFromFloat(0.7))))

	t.Log("2!",
		math.Pow(0, 0.3),
		decimal.Zero.Pow(decimal.NewFromFloat(0.3)),
	)

}

func TestSyncer_getStakeScore(t *testing.T) {
	expect := func(ret bool, errMsg string) {
		if !ret {
			t.Error(errMsg)
		}
	}
	s := Syncer{}
	// 1 x days
	expect(s.getStakeScore(0, 86400, decimal.NewFromInt(1)).Equals(decimal.NewFromInt(1)), "1")
	expect(s.getStakeScore(0, 86300, decimal.NewFromInt(1)).Equals(decimal.NewFromInt(1)), "2")
	expect(s.getStakeScore(0, 86400*2, decimal.NewFromInt(2)).Equals(decimal.NewFromInt(4)), "3")
}
