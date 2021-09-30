package syncer

import (
	"context"
	"errors"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"math"
	"testing"
)

var TEST_ERROR = errors.New("test error")

type MockBlockGraph struct{}

func (mockBlock *MockBlockGraph) GetTimestampToBlockNumber(timestamp int64) (int64, error) {
	// 60 second for 1 block:
	// timestamp 0~59 return 0, timestamp 60~119 return 1
	return timestamp / 60, nil
}

func NewMockBlockGraph() *MockBlockGraph {
	return &MockBlockGraph{}
}

type MockMAI3Graph struct{}

func (mockMAI3 *MockMAI3Graph) GetUsersBasedOnBlockNumber(blockNumber int64) ([]graph.User, error) {
	if blockNumber == 0 {
		return []graph.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(0),
				TotalFee:       decimal.NewFromInt(0),
				UnlockMCBTime:  0,
				MarginAccounts: []*graph.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 1 {
		return []graph.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(3),
				TotalFee:       decimal.NewFromInt(0),
				UnlockMCBTime:  60 * 60 * 24 * 100, // 100 days
				MarginAccounts: []*graph.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 2 {
		return []graph.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(3),
				TotalFee:      decimal.NewFromInt(0),
				UnlockMCBTime: 60 * 60 * 24 * 99, // 99 days
				MarginAccounts: []*graph.MarginAccount{
					{
						ID:       "0xPool-0-0xUser1",
						Position: decimal.NewFromFloat(2),
					},
					{
						ID:       "0xPool-1-0xUser1",
						Position: decimal.NewFromFloat(4),
					},
				},
			},
		}, nil
	}
	if blockNumber == 3 {
		return []graph.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(3),
				TotalFee:      decimal.NewFromInt(10),
				UnlockMCBTime: 60 * 60 * 24 * 98, // 98 days
				MarginAccounts: []*graph.MarginAccount{
					{
						ID:       "0xPool-0-0xUser1",
						Position: decimal.NewFromFloat(2),
					},
					{
						ID:       "0xPool-1-0xUser1",
						Position: decimal.NewFromFloat(4),
					},
				},
			},
		}, nil
	}
	if blockNumber == 4 {
		return []graph.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(10),
				TotalFee:      decimal.NewFromInt(15),
				UnlockMCBTime: 60 * 60 * 24 * 100, // 100 days
				MarginAccounts: []*graph.MarginAccount{
					{
						ID:       "0xPool-0-0xUser1",
						Position: decimal.NewFromFloat(7),
					},
					{
						ID:       "0xPool-1-0xUser1",
						Position: decimal.NewFromFloat(9),
					},
				},
			},
		}, nil
	}
	return []graph.User{}, TEST_ERROR
}

var retMap0 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(0),
	"0xPool-1": decimal.NewFromInt(0),
}

var retMap1 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(100),
	"0xPool-1": decimal.NewFromInt(1000),
}

var retMap2 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(110),
	"0xPool-1": decimal.NewFromInt(1100),
}
var retMap3 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(90),
	"0xPool-1": decimal.NewFromInt(900),
}
var retMap4 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(100),
	"0xPool-1": decimal.NewFromInt(1000),
}

func (mockMAI3 *MockMAI3Graph) GetMarkPrices(blockNumber int64) (map[string]decimal.Decimal, error) {
	if blockNumber == 0 {
		return retMap0, nil
	}
	if blockNumber == 1 {
		return retMap1, nil
	}
	if blockNumber == 2 {
		return retMap2, nil
	}
	if blockNumber == 3 {
		return retMap3, nil
	}
	if blockNumber == 4 {
		return retMap4, nil
	}
	return map[string]decimal.Decimal{}, TEST_ERROR
}

func (mockMAI3 *MockMAI3Graph) GetMarkPriceWithBlockNumberAddrIndex(blockNumber int64, poolAddr string, perpetualIndex int) (decimal.Decimal, error) {
	if blockNumber == 0 {
		return decimal.Zero, nil
	}
	if blockNumber == 1 {
		if perpetualIndex == 0 {
			return retMap1["0xPool-0"], nil
		} else if perpetualIndex == 1 {
			return retMap1["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 2 {
		if perpetualIndex == 0 {
			return retMap2["0xPool-0"], nil
		} else if perpetualIndex == 1 {
			return retMap2["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 3 {
		if perpetualIndex == 0 {
			return retMap3["0xPool-0"], nil
		} else if perpetualIndex == 1 {
			return retMap3["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 4 {
		if perpetualIndex == 0 {
			return retMap4["0xPool-0"], nil
		} else if perpetualIndex == 1 {
			return retMap4["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	return decimal.Zero, TEST_ERROR
}

func NewMockMAI3Graph() *MockMAI3Graph {
	return &MockMAI3Graph{}
}

type SyncerTestSuite struct {
	suite.Suite

	syncer *Syncer
	cancel context.CancelFunc
}

func (t *SyncerTestSuite) SetupSuite() {
	database.Initialize()
	database.Reset(database.GetDB(), types.Watcher, true)
	logger := logging.NewLoggerTag("test suite syncer")
	ctx, cancel := context.WithCancel(context.Background())
	t.syncer = &Syncer{
		logger: logger,
		ctx:    ctx,
		curEpochConfig: &mining.Schedule{
			Epoch:     0,
			StartTime: 0,
			EndTime:   250,
			WeightFee: decimal.NewFromFloat(0.7),
			WeightMCB: decimal.NewFromFloat(0.3),
			WeightOI:  decimal.NewFromFloat(0.3),
		},
		blockGraphInterface: NewMockBlockGraph(),
		mai3GraphInterface:  NewMockMAI3Graph(),
		db:                  database.GetDB(),
	}
	t.cancel = cancel
}

func (t *SyncerTestSuite) TearDownSuite() {
	t.cancel()
	database.DeleteAllData(types.Watcher)
	database.Finalize()
}

func (t *SyncerTestSuite) TestState() {
	var progress mining.Progress
	var users []mining.UserInfo
	np := int64(0)

	err := t.syncer.initUserStates()
	t.Require().Equal(err, nil)
	t.Require().Equal(progress.From, np)
	for np < t.syncer.curEpochConfig.EndTime {
		p, err := t.syncer.syncState()
		t.Require().Equal(err, nil)

		// check progress
		err = t.syncer.db.Model(&mining.Progress{}).Where("table_name = 'user_info' and epoch = 0").First(&progress).Error
		t.Require().Equal(err, nil)
		t.Require().Equal(progress.From, p)

		// check calculation result
		err = t.syncer.db.Model(&mining.UserInfo{}).Where("epoch = 0").Scan(&users).Error
		t.Require().Equal(err, nil)
		if p == 60 {
			// p == 60 -> block == 1
			// stakedMCB 3 * unlockTime 100 == 300
			t.Require().Equal(len(users), 1)
			t.Require().Equal(users[0].CurStakeScore.String(), decimal.NewFromInt(300).String())
		}
		if p == 120 {
			// p == 120 -> block == 2
			// stakedMCB 3 * unlockTime 99 == 99*3
			// position 2*110 + 4*1100 = 4620
			t.Require().Equal(len(users), 1)
			t.Require().Equal(users[0].AccStakeScore.String(), decimal.NewFromInt(300).String())
			t.Require().Equal(users[0].CurStakeScore.String(), decimal.NewFromInt(99*3).String())
			t.Require().Equal(users[0].CurPosValue.String(), decimal.NewFromInt(4620).String())
		}
		if p == 180 {
			// p == 180 -> block == 3
			// stackedMCB 3 * unlockTime 98 == 98*3
			// position 2*90 + 4*900 = 3780
			// fee 10
			// elapsed (time 180 - 0) / 60 == 3
			// score math.pow(10, 0.7), stake math.pow((300+99*3+98*3)/elapsed, 0.3), oi math.pow((4620+3780)/elapsed, 0.3)
			t.Require().Equal(len(users), 1)
			t.Require().Equal(users[0].AccStakeScore.String(), decimal.NewFromInt(300+99*3).String())
			t.Require().Equal(users[0].CurStakeScore.String(), decimal.NewFromInt(98*3).String())
			t.Require().Equal(users[0].AccPosValue.String(), decimal.NewFromInt(4620).String())
			t.Require().Equal(users[0].CurPosValue.String(), decimal.NewFromInt(3780).String())
			t.Require().Equal(users[0].AccFee.String(), decimal.NewFromInt(10).String())
			score := math.Pow(10.0, 0.7) * math.Pow((300.0+99.0*3.0+98.0*3.0)/3.0, 0.3) * math.Pow((4620.0+3780.0)/3.0, 0.3)
			actualScore, _ := users[0].Score.Float64()
			t.Require().Equal(actualScore, score)
		}
		if p == 240 {
			// p == 240 -> block == 4
			// stackedMCB 10 * unlockTime 100 == 1000
			// position 7*100 + 9*1000 = 9700
			// fee 10
			// elapsed (240 - 0) / 60 == 3
			t.Require().Equal(len(users), 1)
			t.Require().Equal(users[0].AccStakeScore.String(), decimal.NewFromInt(300+99*3+98*3).String())
			t.Require().Equal(users[0].CurStakeScore.String(), decimal.NewFromInt(1000).String())
			t.Require().Equal(users[0].AccPosValue.String(), decimal.NewFromInt(4620+3780).String())
			t.Require().Equal(users[0].CurPosValue.String(), decimal.NewFromInt(9700).String())
			t.Require().Equal(users[0].AccFee.String(), decimal.NewFromInt(15).String())
			score := math.Pow(15.0, 0.7) * math.Pow(
				(300.0+99.0*3.0+98.0*3.0+1000)/4.0, 0.3) * math.Pow(
				(4620.0+3780.0+9700.0)/4.0, 0.3)
			actualScore, _ := users[0].Score.Float64()
			t.Require().Equal(actualScore, score)
		}
		np = p + 60
	}
}

func (t *SyncerTestSuite) TestStakeGetScore() {
	// one day
	stakeScore := t.syncer.getStakeScore(0, 86400, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(1))

	// less than one day
	stakeScore = t.syncer.getStakeScore(0, 86300, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(1))

	// more than one day
	stakeScore = t.syncer.getStakeScore(0, 86401, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(2))

	// two day
	stakeScore = t.syncer.getStakeScore(0, 86400*2, decimal.NewFromInt(2))
	t.Require().Equal(stakeScore, decimal.NewFromInt(4))

	stakeScore = t.syncer.getStakeScore(0, 86400*2, decimal.NewFromFloat(1.5))
	t.Require().Equal(stakeScore.String(), decimal.NewFromInt(3).String())
}

func (t *SyncerTestSuite) TestGetScore() {
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
	actual := t.syncer.getScore(&ui, elapse)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitFee:       decimal.NewFromFloat(5),
		AccFee:        decimal.NewFromFloat(213),
		AccPosValue:   decimal.NewFromFloat(0),
		CurPosValue:   decimal.NewFromFloat(0),
		AccStakeScore: decimal.NewFromFloat(3.5),
		CurStakeScore: decimal.NewFromFloat(3),
	}
	actual = t.syncer.getScore(&ui, elapse)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitFee:       decimal.NewFromFloat(5),
		AccFee:        decimal.NewFromFloat(56),
		AccPosValue:   decimal.NewFromFloat(12345),
		CurPosValue:   decimal.NewFromFloat(12),
		AccStakeScore: decimal.NewFromFloat(0),
		CurStakeScore: decimal.NewFromFloat(0),
	}
	actual = t.syncer.getScore(&ui, elapse)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitFee:       decimal.NewFromFloat(2.5),
		AccFee:        decimal.NewFromFloat(5),
		AccPosValue:   decimal.NewFromFloat(4.5),
		CurPosValue:   decimal.NewFromFloat(4),
		AccStakeScore: decimal.NewFromFloat(3.5),
		CurStakeScore: decimal.NewFromFloat(3),
	}
	actual = t.syncer.getScore(&ui, elapse)
	// pow((5-2.5), 0.7) = 1.8991444823309347
	// pow((3.5+3)/2, 0.3) = 1.4241804121672974
	// pow((4.5+4)/2, 0.3) = 1.543535701445671
	// 1.8991444823309347 * 1.4241804121672974 * 1.543535701445671 = 4.174838630152279
	t.Require().Equal(actual.String(), decimal.NewFromFloat(4.174838630152278).String())
}

func TestSyncer(t *testing.T) {
	suite.Run(t, new(SyncerTestSuite))
}
