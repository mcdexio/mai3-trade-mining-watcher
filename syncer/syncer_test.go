package syncer

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"math"
	"testing"
)

type SyncerTestSuite struct {
	suite.Suite

	s      *MockSyncer
	cancel context.CancelFunc
}

func NewMockSyncer(syncer *Syncer) *MockSyncer {
	return &MockSyncer{
		syncer: syncer,
	}
}

type MockSyncer struct {
	mock.Mock
	syncer *Syncer
}

func (mock *MockSyncer) getProgress(name string, epoch int64) (int64, error) {
	args := mock.Called(name, epoch)
	return int64(args.Int(0)), args.Error(0)
}

func (mock *MockSyncer) getStakeScore(curTime int64, unlockTime int64, staked decimal.Decimal) decimal.Decimal {
	return mock.syncer.getStakeScore(curTime, unlockTime, staked)
}

func (mock *MockSyncer) getScore(ui *mining.UserInfo, elapsed decimal.Decimal) decimal.Decimal {
	return mock.syncer.getScore(ui, elapsed)
}

func (t *SyncerTestSuite) SetupSuite() {
	logger := logging.NewLoggerTag("test suite syncer")
	ctx, cancel := context.WithCancel(context.Background())
	syncer := NewSyncer(ctx, logger, "", "", 0)
	t.s = NewMockSyncer(syncer)
	t.cancel = cancel
}

func (t *SyncerTestSuite) TearDownSuite() {
	t.cancel()
}

func (t *SyncerTestSuite) TestStakeGetScore() {
	// one day
	stakeScore := t.s.getStakeScore(0, 86400, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(1))

	// less than one day
	stakeScore = t.s.getStakeScore(0, 86300, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(1))

	// more than one day
	stakeScore = t.s.getStakeScore(0, 86401, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(2))

	// two day
	stakeScore = t.s.getStakeScore(0, 86400*2, decimal.NewFromInt(2))
	t.Require().Equal(stakeScore, decimal.NewFromInt(4))

	stakeScore = t.s.getStakeScore(0, 86400*2, decimal.NewFromFloat(1.5))
	t.Require().Equal(stakeScore.String(), decimal.NewFromInt(3).String())
}

func (t *SyncerTestSuite) TestGetScore() {
	t.s.syncer.curEpochConfig = &mining.Schedule{
		WeightMCB: decimal.NewFromFloat(0.3),
		WeightFee: decimal.NewFromFloat(0.7),
		WeightOI:  decimal.NewFromFloat(0.3),
	}
	minuteCeil := int64(math.Ceil((100.0 - 30.0) / 60.0))
	elapse := decimal.NewFromInt(minuteCeil) // 100 seconds -> 2 minutes

	ui := mining.UserInfo{
		InitFee:       decimal.NewFromFloat(5),
		AccFee:        decimal.NewFromFloat(5),
		AccPosValue:   decimal.NewFromFloat(4.5),
		CurPosValue:   decimal.NewFromFloat(4),
		AccStakeScore: decimal.NewFromFloat(3.5),
		CurStakeScore: decimal.NewFromFloat(3),
	}
	actual := t.s.getScore(&ui, elapse)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitFee:       decimal.NewFromFloat(5),
		AccFee:        decimal.NewFromFloat(213),
		AccPosValue:   decimal.NewFromFloat(0),
		CurPosValue:   decimal.NewFromFloat(0),
		AccStakeScore: decimal.NewFromFloat(3.5),
		CurStakeScore: decimal.NewFromFloat(3),
	}
	actual = t.s.getScore(&ui, elapse)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitFee:       decimal.NewFromFloat(5),
		AccFee:        decimal.NewFromFloat(56),
		AccPosValue:   decimal.NewFromFloat(12345),
		CurPosValue:   decimal.NewFromFloat(12),
		AccStakeScore: decimal.NewFromFloat(0),
		CurStakeScore: decimal.NewFromFloat(0),
	}
	actual = t.s.getScore(&ui, elapse)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitFee:       decimal.NewFromFloat(2.5),
		AccFee:        decimal.NewFromFloat(5),
		AccPosValue:   decimal.NewFromFloat(4.5),
		CurPosValue:   decimal.NewFromFloat(4),
		AccStakeScore: decimal.NewFromFloat(3.5),
		CurStakeScore: decimal.NewFromFloat(3),
	}
	actual = t.s.getScore(&ui, elapse)
	// pow((5-2.5), 0.7) = 1.8991444823309347
	// pow((3.5+3)/2, 0.3) = 1.4241804121672974
	// pow((4.5+4)/2, 0.3) = 1.543535701445671
	// 1.8991444823309347 * 1.4241804121672974 * 1.543535701445671 = 4.174838630152279
	t.Require().Equal(actual.String(), decimal.NewFromFloat(4.174838630152278).String())
}

func TestSyncer(t *testing.T) {
	suite.Run(t, new(SyncerTestSuite))
}
