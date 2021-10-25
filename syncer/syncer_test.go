package syncer

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"math"
	"testing"
)

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
		logger:           logger,
		ctx:              ctx,
		db:               database.GetDB(),
		snapshotInterval: 3600,
	}
	t.cancel = cancel
}

func (t *SyncerTestSuite) TearDownSuite() {
	t.cancel()
	database.DeleteAllData(types.Watcher)
	database.Finalize()
}

func (t *SyncerTestSuite) TestStateOnlyOneChain() {
	// setup blockGraphs, mai3Graphs
	t.syncer.blockGraphs = NewMockMultiBlockGraphs()
	t.syncer.mai3Graphs = NewMockMultiMAI3Graphs()

	var progress mining.Progress
	var users []mining.UserInfo
	np := int64(0)

	epoch := &mining.Schedule{
		Epoch:     0,
		StartTime: 0,
		EndTime:   250,
		WeightFee: decimal.NewFromFloat(0.7),
		WeightMCB: decimal.NewFromFloat(0.3),
		WeightOI:  decimal.NewFromFloat(0.3),
	}

	db := t.syncer.db
	db.Model(&mining.Schedule{}).Delete("epoch=0")
	db.Model(&mining.UserInfo{}).Delete("epoch=0")
	defer database.DeleteAllData(types.Watcher)

	err := t.syncer.initUserStates(t.syncer.db, epoch)
	t.Require().Equal(err, nil)
	t.Require().Equal(progress.From, np)
	totalEpochMinutes := math.Ceil(float64(epoch.EndTime-epoch.StartTime) / 60)
	for np < epoch.EndTime {
		p, err := t.syncer.syncState(t.syncer.db, epoch)
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
			// A = (1 - Floor(RemainEpochSeconds / 86400) / UnlockTimeInDays / 2) * CurrentStakingReward * RemainEpochMinutes
			// remainEpochDays 0, remainProportion 1, remainEpochMinutes 4, A = (1 - 0) * 300 * 4 = 1200
			t.Require().Equal(users[0].EstimatedStakeScore.String(), decimal.NewFromInt(1200).String())
		}
		if p == 120 {
			// p == 120 -> block == 2
			// stakedMCB 3 * unlockTime 99 == 99*3
			// position 2*110 + 4*1100 = 4620
			t.Require().Equal(len(users), 1)
			t.Require().Equal(users[0].AccStakeScore.String(), decimal.NewFromInt(300).String())
			t.Require().Equal(users[0].CurStakeScore.String(), decimal.NewFromInt(99*3).String())
			t.Require().Equal(users[0].CurPosValue.String(), decimal.NewFromInt(4620).String())
			// remainEpochDays 0, remainProportion 1, remainEpochMinutes 3, A = (1 - 0) * 99*3 * 3 = 891
			t.Require().Equal(users[0].EstimatedStakeScore.String(), decimal.NewFromInt(891).String())
		}
		if p == 180 {
			// p == 180 -> block == 3
			// stackedMCB 3 * unlockTime 98 == 98*3
			// position 2*90 + 4*900 = 3780
			// fee 10
			// elapsed (time 180 - 0) / 60 == 3
			// score math.pow(10, 0.7), stake math.pow((300+99*3+98*3+estimatedScore)/totalTime, 0.3), oi math.pow((4620+3780*remains)/totalTime, 0.3)

			t.Require().Equal(len(users), 1)
			t.Require().Equal(users[0].AccStakeScore.String(), decimal.NewFromInt(300+99*3).String())
			t.Require().Equal(users[0].CurStakeScore.String(), decimal.NewFromInt(98*3).String())
			t.Require().Equal(users[0].AccPosValue.String(), decimal.NewFromInt(4620).String())
			t.Require().Equal(users[0].CurPosValue.String(), decimal.NewFromInt(3780).String())
			t.Require().Equal(users[0].AccTotalFee.String(), decimal.NewFromInt(10).String())
			// remainEpochDays 0, remainProportion 1, remainEpochMinutes 2, A = (1 - 0) * 98*3 * 2 = 588
			t.Require().Equal(users[0].EstimatedStakeScore.String(), decimal.NewFromInt(588).String())

			minuteCeil := int64(math.Floor((float64(users[0].Timestamp) - float64(epoch.StartTime)) / 60.0))
			remains := decimal.NewFromInt((epoch.EndTime-epoch.StartTime)/60.0 - minuteCeil) // total epoch in minutes
			remainsFloat, _ := remains.Float64()

			score := math.Pow(10.0, 0.7) * math.Pow((300.0+99.0*3.0+98.0*3.0+588)/totalEpochMinutes, 0.3) * math.Pow((4620.0+3780.0*remainsFloat)/totalEpochMinutes, 0.3)
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
			t.Require().Equal(users[0].AccTotalFee.String(), decimal.NewFromInt(15).String())
			// remainEpochDays 0, remainProportion 1, remainEpochMinutes 2, A = (1 - 0) * 100*10 * 1 = 1000
			t.Require().Equal(users[0].EstimatedStakeScore.String(), decimal.NewFromInt(1000).String())

			minuteCeil := int64(math.Floor((float64(users[0].Timestamp) - float64(epoch.StartTime)) / 60.0))
			remains := decimal.NewFromInt((epoch.EndTime-epoch.StartTime)/60.0 - minuteCeil) // total epoch in minutes
			remainsFloat, _ := remains.Float64()

			score := math.Pow(15.0, 0.7) * math.Pow(
				(300.0+99.0*3.0+98.0*3.0+1000+1000)/totalEpochMinutes, 0.3) * math.Pow(
				(4620.0+3780.0+9700.0*remainsFloat)/totalEpochMinutes, 0.3)
			actualScore, _ := users[0].Score.Float64()
			t.Require().Equal(actualScore, score)
		}
		np = p + 60
	}
}

func (t *SyncerTestSuite) TestGetStakeScore() {
	// one day
	stakeScore := getStakeScore(0, 86400, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(1))

	// less than one day
	stakeScore = getStakeScore(0, 86300, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(1))

	// more than one day
	stakeScore = getStakeScore(0, 86401, decimal.NewFromInt(1))
	t.Require().Equal(stakeScore, decimal.NewFromInt(2))

	// two day
	stakeScore = getStakeScore(0, 86400*2, decimal.NewFromInt(2))
	t.Require().Equal(stakeScore, decimal.NewFromInt(4))

	stakeScore = getStakeScore(0, 86400*2, decimal.NewFromFloat(1.5))
	t.Require().Equal(stakeScore.String(), decimal.NewFromInt(3).String())
}

func (t *SyncerTestSuite) TestGetEstimatedStakeScore() {
	epoch := &mining.Schedule{
		StartTime: 0,
		EndTime:   250,
	}
	// remainEpochDays floor((250 - 125) / 86400) = 0, unlockTimeInDays ceil((300-250)/ 86400) = 1
	// remainEpochMinutes ceil((250-125)/60) = 3
	// estimatedSS 1 * 50 * 3 = 150
	estimatedSS := getEstimatedStakeScore(125, epoch, 300, decimal.NewFromInt(50))
	t.Require().Equal(estimatedSS, decimal.NewFromInt(150))

	epoch = &mining.Schedule{
		StartTime: 10,
		EndTime:   87400,
	}
	// remainEpochDays floor((87400 - 125) / 86400) = 1, unlockTimeInDays ceil((90000-125)/ 86400) = 2
	// remainProportion = 1 - (1/2/2) = 0.75
	// remainEpochMinutes ceil((87400-125)/60) = 1455
	// estimatedSS 0.75 * 20 * 1455
	estimatedSS = getEstimatedStakeScore(125, epoch, 90000, decimal.NewFromInt(20))
	t.Require().Equal(estimatedSS.String(), decimal.NewFromInt(21825).String())

	// endTime == nowTS
	epoch = &mining.Schedule{
		StartTime: 10,
		EndTime:   250,
	}
	estimatedSS = getEstimatedStakeScore(250, epoch, 300, decimal.NewFromInt(50))
	t.Require().Equal(estimatedSS, decimal.Zero)

	// endTime < nowTS
	epoch = &mining.Schedule{
		StartTime: 10,
		EndTime:   250,
	}
	estimatedSS = getEstimatedStakeScore(255, epoch, 300, decimal.NewFromInt(50))
	t.Require().Equal(estimatedSS, decimal.Zero)

	// unlockTime < nowTS
	epoch = &mining.Schedule{
		StartTime: 10,
		EndTime:   87400,
	}
	estimatedSS = getEstimatedStakeScore(85000, epoch, 80000, decimal.NewFromInt(20))
	t.Require().Equal(estimatedSS, decimal.Zero)

	// unlockTime == nowTS
	epoch = &mining.Schedule{
		StartTime: 10,
		EndTime:   87400,
	}
	estimatedSS = getEstimatedStakeScore(80000, epoch, 80000, decimal.NewFromInt(20))
	t.Require().Equal(estimatedSS, decimal.Zero)

	// nowTS < unlockTime < endTime
	epoch = &mining.Schedule{
		StartTime: 10,
		EndTime:   87400,
	}
	// remainEpochDays floor((87400 - 9999) / 86400) = 0, unlockTimeInDays ceil((80000-9999)/ 86400) = 1
	// remainProportion = 1 - (0/1/2) = 1
	// remainEpochMinutes ceil((87400-9999)/60) = 1291
	// estimatedSS 1 * 125 * 1291
	estimatedSS = getEstimatedStakeScore(9999, epoch, 80000, decimal.NewFromInt(125))
	t.Require().Equal(estimatedSS.String(), decimal.NewFromInt(161375).String())
}

func (t *SyncerTestSuite) TestGetScore() {
	epoch := &mining.Schedule{
		Epoch:     0,
		StartTime: 30,
		EndTime:   250,
		WeightFee: decimal.NewFromFloat(0.7),
		WeightMCB: decimal.NewFromFloat(0.3),
		WeightOI:  decimal.NewFromFloat(0.3),
	}
	// now 100, start 30
	minuteCeil := int64(math.Ceil((100.0 - 30.0) / 60.0))
	remains := decimal.NewFromInt((epoch.EndTime-epoch.StartTime)/60.0 - minuteCeil) // total epoch in minutes

	ui := mining.UserInfo{
		InitTotalFee:  decimal.NewFromFloat(5),
		AccTotalFee:   decimal.NewFromFloat(5),
		AccPosValue:   decimal.NewFromFloat(4.5),
		CurPosValue:   decimal.NewFromFloat(4),
		AccStakeScore: decimal.NewFromFloat(3.5),
		CurStakeScore: decimal.NewFromFloat(3),
	}
	actual := getScore(epoch, &ui, remains)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitTotalFee:  decimal.NewFromFloat(5),
		AccTotalFee:   decimal.NewFromFloat(213),
		AccPosValue:   decimal.NewFromFloat(0),
		CurPosValue:   decimal.NewFromFloat(0),
		AccStakeScore: decimal.NewFromFloat(3.5),
		CurStakeScore: decimal.NewFromFloat(3),
	}
	actual = getScore(epoch, &ui, remains)
	t.Require().Equal(actual, decimal.Zero)

	ui = mining.UserInfo{
		InitTotalFee:  decimal.NewFromFloat(5),
		AccTotalFee:   decimal.NewFromFloat(56),
		AccPosValue:   decimal.NewFromFloat(12345),
		CurPosValue:   decimal.NewFromFloat(12),
		AccStakeScore: decimal.NewFromFloat(0),
		CurStakeScore: decimal.NewFromFloat(0),
	}
	actual = getScore(epoch, &ui, remains)
	t.Require().Equal(actual, decimal.Zero)

	currentStakeReward := decimal.NewFromFloat(3)
	estimatedStakeScore := getEstimatedStakeScore(100, epoch, 60*60*24*100, currentStakeReward)
	ui = mining.UserInfo{
		InitTotalFee:        decimal.NewFromFloat(2.5),
		AccTotalFee:         decimal.NewFromFloat(5),
		AccPosValue:         decimal.NewFromFloat(4.5),
		CurPosValue:         decimal.NewFromFloat(4),
		AccStakeScore:       decimal.NewFromFloat(3.5),
		EstimatedStakeScore: estimatedStakeScore,
		CurStakeScore:       currentStakeReward,
	}
	actual = getScore(epoch, &ui, remains)
	// pow((5-2.5), 0.7) = 1.8991444823309347
	// pow(((3.5+3+9)/4), 0.3) = 1.5013484918805586
	// pow((4.5+4*1)/4, 0.3) = 1.2537405723606492
	// 1.8991444823309347 * 1.5013484918805586 * 1.2537405723606492 = 4.4010489315
	t.Require().Equal(actual.String(), decimal.NewFromFloat(3.5747625410871953).String())
}

func (t *SyncerTestSuite) TestDetectEpoch() {
	epoch := &mining.Schedule{
		Epoch:     0,
		StartTime: 100,
		EndTime:   250,
		WeightFee: decimal.NewFromFloat(0.7),
		WeightMCB: decimal.NewFromFloat(0.3),
		WeightOI:  decimal.NewFromFloat(0.3),
	}
	db := t.syncer.db
	t.Require().Equal(nil, db.Model(&mining.Schedule{}).Create(&epoch).Error)
	defer database.DeleteAllData(types.Watcher)
	// before
	{
		e, err := t.syncer.detectEpoch(db, 9)
		t.Require().Equal(nil, err)
		t.Require().Equal(epoch.Epoch, e.Epoch)
		t.Require().Equal(epoch.StartTime, e.StartTime)
		t.Require().Equal(epoch.EndTime, e.EndTime)
	}
	// in
	{
		e, err := t.syncer.detectEpoch(db, 110)
		t.Require().Equal(nil, err)
		t.Require().Equal(epoch.Epoch, e.Epoch)
		t.Require().Equal(epoch.StartTime, e.StartTime)
		t.Require().Equal(epoch.EndTime, e.EndTime)
	}
	// edge case
	{
		e, err := t.syncer.detectEpoch(db, 100)
		t.Require().Equal(nil, err)
		t.Require().Equal(epoch.Epoch, e.Epoch)
		t.Require().Equal(epoch.StartTime, e.StartTime)
		t.Require().Equal(epoch.EndTime, e.EndTime)
	}
	// edge case
	{
		_, err := t.syncer.detectEpoch(db, 190) // + 60 == end
		t.Require().Equal("not in epoch period", err.Error())
	}
}

func (t *SyncerTestSuite) TestRestoreFromSnapshot() {
	// setup blockGraphs, mai3Graphs
	t.syncer.blockGraphs = NewMockMultiBlockGraphs()
	t.syncer.mai3Graphs = NewMockMultiMAI3Graphs()

	epoch := &mining.Schedule{
		Epoch:     0,
		StartTime: 0,
		EndTime:   250,
		WeightFee: decimal.NewFromFloat(0.7),
		WeightMCB: decimal.NewFromFloat(0.3),
		WeightOI:  decimal.NewFromFloat(0.3),
	}
	db := t.syncer.db
	db.Model(&mining.Schedule{}).Delete("epoch=0")
	t.Require().Equal(nil, db.Model(&mining.Schedule{}).Create(&epoch).Error)
	defer database.DeleteAllData(types.Watcher)

	t.syncer.snapshotInterval = 120
	defer func() {
		t.syncer.snapshotInterval = 3600
	}()
	// before
	{
		t.syncer.syncState(t.syncer.db, epoch)         // 60, 100
		t.syncer.syncState(t.syncer.db, epoch)         // 120, 99
		p, _ := t.syncer.syncState(t.syncer.db, epoch) // 180, 98
		t.Require().Equal(int64(180), p)
	}
	// block == 3
	var user mining.UserInfo
	var progress mining.Progress
	t.syncer.db.Model(&mining.UserInfo{}).Where("epoch = 0").First(&user)
	t.Require().Equal(decimal.NewFromInt(294).String(), user.CurStakeScore.String())
	t.Require().Equal(decimal.NewFromInt(300+297).String(), user.AccStakeScore.String())
	t.syncer.db.Model(&mining.Progress{}).Where("table_name = 'user_info' and epoch = 0").First(&progress)
	t.Require().Equal(int64(180), progress.From)

	var snapshots []*mining.Snapshot
	db.Where("epoch=? and timestamp=?", epoch.Epoch, 120).Find(&snapshots)
	t.Require().Equal(1, len(snapshots))
	t.Require().Equal(decimal.NewFromInt(297).String(), snapshots[0].CurStakeScore.String())
	t.Require().Equal(decimal.NewFromInt(300).String(), snapshots[0].AccStakeScore.String())

	t.syncer.restoreFromSnapshot(db, 120)
	t.syncer.db.Model(&mining.UserInfo{}).Where("epoch = 0").First(&user)
	t.Require().Equal(decimal.NewFromInt(297).String(), user.CurStakeScore.String())
	t.Require().Equal(decimal.NewFromInt(300).String(), user.AccStakeScore.String())
	t.syncer.db.Model(&mining.Progress{}).Where("table_name = 'user_info' and epoch = 0").First(&progress)
	t.Require().Equal(int64(120), progress.From)
	{
		p, err := t.syncer.syncState(t.syncer.db, epoch) // 180, 98
		t.Require().Equal(nil, err)
		t.Require().Equal(int64(180), p)
	}
	// again
	t.syncer.db.Model(&mining.Progress{}).Where("table_name = 'user_info' and epoch = 0").First(&progress)
	t.Require().Equal(int64(180), progress.From)

	err := t.syncer.runRestore(context.Background(), 120)
	t.Require().Equal(nil, err)

	t.syncer.db.Model(&mining.Progress{}).Where("table_name = 'user_info' and epoch = 0").First(&progress)
	t.Require().Equal(int64(240), progress.From)

	t.syncer.db.Model(&mining.UserInfo{}).Where("epoch = 0").First(&user)
	t.Require().Equal(decimal.NewFromInt(1000).String(), user.CurStakeScore.String())
	t.Require().Equal(decimal.NewFromInt(300+297+294).String(), user.AccStakeScore.String())
}

// func (t *SyncerTestSuite) TestRestoreTransaction() {
// 	epoch := &mining.Schedule{
// 		Epoch:     0,
// 		StartTime: 0,
// 		EndTime:   250,
// 		WeightFee: decimal.NewFromFloat(0.7),
// 		WeightMCB: decimal.NewFromFloat(0.3),
// 		WeightOI:  decimal.NewFromFloat(0.3),
// 	}
// 	db := t.syncer.db
// 	db.Model(&mining.Schedule{}).Delete("epoch=0")
// 	t.Require().Equal(nil, db.Model(&mining.Schedule{}).Create(&epoch).Error)
// 	defer database.DeleteAllData(types.Watcher)
//
// 	t.syncer.snapshotInterval = 120
// 	defer func() {
// 		t.syncer.snapshotInterval = 3600
// 	}()
// 	// before
// 	{
// 		t.syncer.syncState(t.syncer.db, epoch)         // 60, 100
// 		t.syncer.syncState(t.syncer.db, epoch)         // 120, 99
// 		p, _ := t.syncer.syncState(t.syncer.db, epoch) // 180, 98
// 		t.Require().Equal(int64(180), p)
// 	}
//
// 	// set mock delay
// 	g, _ := t.syncer.mai3Graph1.(*MockMAI3Graph1)
// 	g.delay = 1 * time.Second
// 	defer func() {
// 		g.delay = 0
// 	}()
// 	assert := func(css, asc, ts int64) {
// 		var user mining.UserInfo
// 		t.syncer.db.Model(&mining.UserInfo{}).Where("epoch = 0").First(&user)
// 		t.Require().Equal(decimal.NewFromInt(css).String(), user.CurStakeScore.String())
// 		t.Require().Equal(decimal.NewFromInt(asc).String(), user.AccStakeScore.String())
// 		t.Require().Equal(user.Timestamp, ts)
// 	}
// 	ch := make(chan bool)
// 	go func() {
// 		for {
// 			select {
// 			case <-ch:
// 				assert(1000, 300+297+294, 240)
// 				return
// 			default:
// 				assert(294, 300+297, 180)
// 			}
// 			time.Sleep(500 * time.Millisecond)
// 		}
// 	}()
//
// 	err := t.syncer.runRestore(context.Background(), 120)
// 	t.Require().Equal(nil, err)
// 	ch <- false
// 	assert(1000, 300+297+294, 240)
//
// 	var progress mining.Progress
// 	t.syncer.db.Model(&mining.Progress{}).Where("table_name = 'user_info' and epoch = 0").First(&progress)
// 	t.Require().Equal(int64(240), progress.From)
// }

func TestSyncer(t *testing.T) {
	suite.Run(t, new(SyncerTestSuite))
}
