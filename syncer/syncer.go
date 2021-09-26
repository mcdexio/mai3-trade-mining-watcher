package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
)

var NOT_IN_EPOCH = errors.New("not in epoch period")
var EMPTY_SCHEDULE = errors.New("empty schedule")
var minuteDecimal = decimal.NewFromInt(60)

var Transport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout: 500 * time.Millisecond,
	}).DialContext,
	TLSHandshakeTimeout: 1000 * time.Millisecond,
	MaxIdleConns:        100,
	IdleConnTimeout:     30 * time.Second,
}

type Syncer struct {
	ctx        context.Context
	httpClient *utils.Client
	logger     logging.Logger
	db         *gorm.DB

	// block syncer
	blockGraphUrl string
	blockSyncer   *BlockSyncer

	mai3GraphUrl string
	blockNumber  int64
	lastTime     int64

	// cache
	initFee map[string]decimal.Decimal
	prevOI  map[string]decimal.Decimal
	prevMCB map[string]decimal.Decimal

	// weight
	thisEpoch          int64
	thisEpochStartTime int64
	thisEpochEndTime   int64
	thisEpochWeightOI  float64
	thisEpochWeightMCB float64
	thisEpochWeightFee float64
}

type MarginAccount struct {
	ID       string          `json:"id"`
	Position decimal.Decimal `json:"position"`
}

type User struct {
	ID             string          `json:"id"`
	StakedMCB      decimal.Decimal `json:"stakedMCB"`
	TotalFee       decimal.Decimal `json:"totalFee"`
	MarginAccounts []*MarginAccount
}

func NewSyncer(
	ctx context.Context, logger logging.Logger, mai3GraphUrl string,
	blockGraphUrl string, blockStartTime *time.Time,
) *Syncer {
	syncer := &Syncer{
		ctx:           ctx,
		httpClient:    utils.NewHttpClient(Transport, logger),
		logger:        logger,
		mai3GraphUrl:  mai3GraphUrl,
		blockGraphUrl: blockGraphUrl,
		blockSyncer:   NewBlockSyncer(ctx, logger, blockGraphUrl, blockStartTime),
		db:            database.GetDB(),
		initFee:       make(map[string]decimal.Decimal),
		prevMCB:       make(map[string]decimal.Decimal),
		prevOI:        make(map[string]decimal.Decimal),
	}
	return syncer
}

func (s *Syncer) setDefaultEpoch() {
	s.thisEpoch = 0
	s.thisEpochStartTime = time.Now().Unix()
	s.thisEpochEndTime = s.thisEpochStartTime + 60*60*24*14
	s.thisEpochWeightMCB = 0.3
	s.thisEpochWeightFee = 0.7
	s.thisEpochWeightOI = 0.3
	err := s.db.Create(
		mining.Schedule{
			Epoch:     s.thisEpoch,
			StartTime: s.thisEpochStartTime,
			EndTime:   s.thisEpochEndTime,
			WeightFee: decimal.NewFromFloat(s.thisEpochWeightFee),
			WeightMCB: decimal.NewFromFloat(s.thisEpochWeightMCB),
			WeightOI:  decimal.NewFromFloat(s.thisEpochWeightOI),
		}).Error
	if err != nil {
		s.logger.Error("set default epoch error %s", err)
		panic(err)
	}
}

func (s *Syncer) Init() {
	s.blockSyncer.Init()
	// get this epoch number, thisEpochStartTime, thisEpochEndTime
	err := s.GetEpoch()
	if err == NOT_IN_EPOCH {
		s.logger.Warn("warn %s", err)
	} else if err == EMPTY_SCHEDULE {
		s.logger.Warn("warn %s", err)
		s.setDefaultEpoch()
	} else if err != nil {
		panic(err)
	}

	s.catchup()
}

func (s *Syncer) Run() error {
	// sync block
	go func() {
		err := s.blockSyncer.Run()
		if err != nil {
			s.logger.Error("block syncer err=%s", err)
			return
		}
	}()

	ticker1min := time.NewTicker(1 * time.Minute)
	ticker1hour := time.NewTicker(1*time.Hour)
	for {
		select {
		case <-s.ctx.Done():
			ticker1min.Stop()
			s.logger.Info("Syncer receives shutdown signal.")
			return nil
		case <-ticker1hour.C:
			s.syncStatePreventRollback()
		case <-ticker1min.C:
			s.syncState()
		}
	}
}

func (s *Syncer) GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error) {
	var retUser []User
	s.logger.Info("get users based on block number %d", blockNumber)
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		users(first: 500, skip: %d, block: { number: %d }, where: {totalFee_gt: 0}) {
			id
			stakedMCB
			totalFee
			marginAccounts(where:{position_gt: 0}){
	  			id
	  			position
			}
		}
	}`
	skip := 0
	for {
		params.Query = fmt.Sprintf(queryFormat, skip, blockNumber)
		err, code, res := s.httpClient.Post(s.mai3GraphUrl, nil, params, nil)
		if err != nil || code != 200 {
			s.logger.Info("Failed to get MAI3 Trading Mining2 info err:%s, code:%d", err, code)
			return nil, err
		}
		var response struct {
			Data struct {
				Users []User
			}
		}
		err = json.Unmarshal(res, &response)
		if err != nil {
			s.logger.Error("Failed to unmarshal err:%s", err)
			return nil, err
		}
		for _, user := range response.Data.Users {
			retUser = append(retUser, user)
		}
		if len(response.Data.Users) == 500 {
			// means there are more data to get
			skip += 500
			if skip == 5000 {
				s.logger.Warn("user more than 500, but we don't have filter")
				break
			}
		} else {
			break
		}
	}
	return retUser, nil
}

func (s *Syncer) TimestampToBlockNumber(startTime int64) (int64, error) {
	s.logger.Debug("transform timestamp %d to block number", startTime)
	var blockInfo mining.Block
	err := s.db.Model(&mining.Block{}).Limit(1).Order(
		"number desc").Where(
		"timestamp < ?", startTime,
	).Scan(&blockInfo).Error
	if err != nil {
		s.logger.Error("Failed to get block info %+v %s", blockInfo, err)
		return -1, err
	}
	return blockInfo.Number, nil
}

func (s *Syncer) syncStatePreventRollback() {
	s.logger.Info("Sync state until one hour ago for preventing rollback")
}

func (s *Syncer) GetPoolAddrIndexUserID(marginAccountID string) (poolAddr, userId string, perpetualIndex int, err error) {
	rest := strings.Split(marginAccountID, "-")
	perpetualIndex, err = strconv.Atoi(rest[1])
	if err != nil {
		return
	}
	poolAddr = rest[0]
	userId = rest[2]
	return
}

func (s *Syncer) calOI(marginAccounts []*MarginAccount, blockNumber int64) decimal.Decimal {
	s.logger.Debug("calculate OI based on marginAccounts and blockNumber")
	if len(marginAccounts) == 0 {
		return decimal.Zero
	}
	sumOI := decimal.Zero
	for _, m := range marginAccounts {
		poolAddr, _, perpetualIndex, err := s.GetPoolAddrIndexUserID(m.ID)
		if err != nil {
			continue
		}
		price, err := s.GetMarkPriceBasedOnBlockNumber(blockNumber, poolAddr, perpetualIndex)
		if err != nil {
			continue
		}
		oi := price.Mul(m.Position)
		sumOI = sumOI.Add(oi)
	}
	return sumOI
}

func (s *Syncer) catchup() {
	var err error
	// depend on this epoch, catchup to now
	s.logger.Info("Catchup stats from %d until now", s.thisEpochStartTime)
	if s.thisEpochStartTime != 0 {
		s.blockSyncer.catchup(s.thisEpochStartTime-60, time.Now().Unix())
	}
	blockNumber, err := s.TimestampToBlockNumber(s.thisEpochStartTime)
	if err != nil {
		s.logger.Error("Failed to get timestampToBlockNumber %d", s.thisEpochStartTime)
		return
	}

	// 1. initFee on blockNumber
	user, err := s.GetUsersBasedOnBlockNumber(blockNumber)
	for _, u := range user {
		preFee := s.getFee(u.ID, blockNumber-1)
		s.initFee[u.ID] = preFee
		oi := s.calOI(u.MarginAccounts, blockNumber)
		s.prevOI[u.ID] = oi
		s.prevMCB[u.ID] = u.StakedMCB
		fee := u.TotalFee.Add(preFee.Neg())
		score := s.calScore(fee, oi, u.StakedMCB)

		var userInfo = &mining.UserInfo{
			Trader:    u.ID,
			Fee:       fee,
			Stake:     u.StakedMCB,
			OI:        oi,
			Score:     score,
			Timestamp: s.thisEpochStartTime,
			Epoch:     s.thisEpoch,
		}
		s.db.Create(userInfo)
	}

	starTimeDecimal := decimal.NewFromInt(s.thisEpochStartTime).Div(minuteDecimal) // to minute
	now := time.Now().Unix()
	nowDecimal := decimal.NewFromInt(now).Div(minuteDecimal) // to minute
	fromStartTimeToNow := nowDecimal.Add(starTimeDecimal.Neg())

	catchUpTime := s.thisEpochStartTime
	// interval := decimal.NewFromInt(1) // 1 minute

	for catchUpTime+60 < now {
		catchUpTimeDecimal := decimal.NewFromInt(catchUpTime).Div(minuteDecimal)
		fromStartTimeToLast := catchUpTimeDecimal.Add(starTimeDecimal.Neg())
		catchUpTime += 60

		blockNumber, err = s.TimestampToBlockNumber(catchUpTime)
		if err != nil {
			s.logger.Error("Failed to get timestampToBlockNumber %d", s.thisEpochStartTime)
			return
		}
		user, err = s.GetUsersBasedOnBlockNumber(blockNumber)
		for _, u := range user {
			var fee decimal.Decimal
			if iFee, match := s.initFee[u.ID]; !match {
				s.initFee[u.ID] = decimal.Zero
			} else {
				fee = u.TotalFee.Add(iFee.Neg()) // this block number(time) - init fee
			}

			var stake decimal.Decimal
			if iStack, match := s.prevMCB[u.ID]; !match {
				s.prevMCB[u.ID] = u.StakedMCB
			} else {
				stake = ((iStack.Mul(fromStartTimeToLast)).Add(u.StakedMCB)).Div(fromStartTimeToNow)
			}

			var oi decimal.Decimal
			thisOI := s.calOI(u.MarginAccounts, blockNumber)
			if iOI, match := s.prevMCB[u.ID]; !match {
				s.prevOI[u.ID] = thisOI
			} else {
				oi = ((iOI.Mul(fromStartTimeToLast)).Add(thisOI)).Div(fromStartTimeToNow)
			}

			score := s.calScore(fee, oi, u.StakedMCB)

			var userInfo = &mining.UserInfo{
				Trader:    u.ID,
				Fee:       fee,
				OI:        oi,    // (oi * 1 + preOI * (lastTime - start)) / now -start
				Stake:     stake, // (s * 1 + preS * (lastTime - start)) / now -start
				Score:     score,
				Timestamp: catchUpTime,
				Epoch:     s.thisEpoch,
			}
			s.db.Create(userInfo)
		}
	}
	s.lastTime = catchUpTime
	return
}

func (s *Syncer) getFee(id string, blockNumber int64) decimal.Decimal {
	s.logger.Debug("get fee from graph based on ID %s, block number %d", id, blockNumber)
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		users(first: 1, block: { number: %d }, where: {id: "%s"}) {
			totalFee
		}
	}`
	params.Query = fmt.Sprintf(queryFormat, blockNumber, id)
	err, code, res := s.httpClient.Post(s.mai3GraphUrl, nil, params, nil)
	if err != nil || code != 200 {
		s.logger.Error("Failed to get MAI3 Trading Mining2 info err:%s, code:%d", err, code)
		return decimal.Zero
	}
	var response struct {
		Data struct {
			Users []User
		}
	}
	err = json.Unmarshal(res, &response)
	if err != nil {
		s.logger.Error("Failed to unmarshal err:%s", err)
	}
	if len(response.Data.Users) == 1 {
		return response.Data.Users[0].TotalFee
	}
	return decimal.Zero
}

func (s *Syncer) syncState() {
	s.logger.Info("Sync state")
	s.lastTime += 60
	blockNumber, err := s.TimestampToBlockNumber(s.lastTime)
	if err != nil {
		s.logger.Error("Failed to get timestampToBlockNumber %d", s.lastTime)
		return
	}
	user, err := s.GetUsersBasedOnBlockNumber(blockNumber)

	starTimeDecimal := decimal.NewFromInt(s.thisEpochStartTime).Div(minuteDecimal) // to minute
	now := time.Now().Unix()
	nowDecimal := decimal.NewFromInt(now).Div(minuteDecimal) // to minute
	fromStartTimeToNow := nowDecimal.Add(starTimeDecimal.Neg())

	lastTimeDecimal := decimal.NewFromInt(s.lastTime).Div(minuteDecimal) // to minute
	fromStartTimeToLast := lastTimeDecimal.Add(starTimeDecimal.Neg())

	for _, u := range user {
		var fee decimal.Decimal
		if iFee, match := s.initFee[u.ID]; !match {
			s.initFee[u.ID] = decimal.Zero
		} else {
			fee = u.TotalFee.Add(iFee.Neg()) // this block number(time) - init fee
		}

		var stake decimal.Decimal
		if iStack, match := s.prevMCB[u.ID]; !match {
			s.prevMCB[u.ID] = u.StakedMCB
		} else {
			stake = ((iStack.Mul(fromStartTimeToLast)).Add(u.StakedMCB)).Div(fromStartTimeToNow)
		}

		var oi decimal.Decimal
		thisOI := s.calOI(u.MarginAccounts, blockNumber)
		if iOI, match := s.prevMCB[u.ID]; !match {
			s.prevOI[u.ID] = thisOI
		} else {
			oi = ((iOI.Mul(fromStartTimeToLast)).Add(thisOI)).Div(fromStartTimeToNow)
		}

		score := s.calScore(fee, oi, u.StakedMCB)

		var userInfo = &mining.UserInfo{
			Trader:    u.ID,
			Fee:       fee,
			OI:        oi,    // (oi * 1 + preOI * (lastTime - start)) / now -start
			Stake:     stake, // (s * 1 + preS * (lastTime - start)) / now -start
			Score:     score,
			Timestamp: s.lastTime,
			Epoch:     s.thisEpoch,
		}
		s.db.Create(userInfo)
	}
	return
}

func (s *Syncer) GetMarkPriceBasedOnBlockNumber(blockNumber int64, poolAddr string, perpetualIndex int) (*decimal.Decimal, error) {
	s.logger.Debug("Get mark price based on block number %d, poolAddr %s, perpetualIndex %d", blockNumber, poolAddr, perpetualIndex)
	id := fmt.Sprintf("%s-%d", poolAddr, perpetualIndex)
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		markPrices(first: 1, block: { number: %d }, where: {id: "%s"}) {
    		id
    		price
    		timestamp
		}
	}`
	params.Query = fmt.Sprintf(queryFormat, blockNumber, id)
	err, code, res := s.httpClient.Post(s.mai3GraphUrl, nil, params, nil)
	if err != nil || code != 200 {
		s.logger.Info("Failed to get MAI3 Trading Mining2 info err:%s, code:%d", err, code)
		return nil, err
	}
	var response struct {
		Data struct {
			MarkPrices []struct {
				ID    string          `json:"id"`
				Price decimal.Decimal `json:"price"`
			}
		}
	}
	err = json.Unmarshal(res, &response)
	if err != nil {
		s.logger.Error("Unmarshal error. err:%s", err)
		return nil, err
	}
	if len(response.Data.MarkPrices) == 0 {
		return nil, errors.New("empty mark price")
	}
	return &response.Data.MarkPrices[0].Price, nil
}

func (s *Syncer) GetEpoch() error {
	// get epoch from schedule database.
	now := time.Now().Unix()
	var schedules []mining.Schedule
	err := s.db.Model(&mining.Schedule{}).Scan(&schedules).Error
	if err != nil {
		s.logger.Error("Failed to get schedule %s", err)
		return err
	}
	if len(schedules) == 0 {
		return EMPTY_SCHEDULE
	}
	for _, schedule := range schedules {
		if now > schedule.StartTime && now < schedule.EndTime {
			s.thisEpoch = schedule.Epoch
			s.thisEpochStartTime = schedule.StartTime
			s.thisEpochEndTime = schedule.EndTime
			s.thisEpochWeightFee, _ = schedule.WeightFee.Float64()
			s.thisEpochWeightOI, _ = schedule.WeightOI.Float64()
			s.thisEpochWeightMCB, _ = schedule.WeightMCB.Float64()
			return nil
		}
	}
	return NOT_IN_EPOCH
}

func (s *Syncer) calScore(fee, oi, stake decimal.Decimal) decimal.Decimal {
	// there are issue on decimal pow, so using float64
	feeInflate, _ := fee.Float64()
	oiInflate, _ := oi.Float64()
	stakeInflate, _ := stake.Float64()
	score := math.Pow(feeInflate, s.thisEpochWeightFee) * math.Pow(oiInflate, s.thisEpochWeightOI) * math.Pow(stakeInflate, s.thisEpochWeightMCB)
	return decimal.NewFromFloat(score)
}
