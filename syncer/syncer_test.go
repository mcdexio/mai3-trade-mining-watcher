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
