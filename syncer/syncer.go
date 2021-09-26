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

	mai3GraphUrl       string
	epoch              int64
	thisEpochStartTime int64
	thisEpochEndTime   int64
	blockNumber        int64

	// weight
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
	}
	return syncer
}

func (s *Syncer) defaultEpoch() {
	s.thisEpochStartTime = time.Now().Unix()
	s.thisEpochEndTime = s.thisEpochEndTime + 60*60*24*14
	s.thisEpochWeightMCB = 0.3
	s.thisEpochWeightFee = 0.7
	s.thisEpochWeightOI = 0.3
	err := s.db.Create(
		mining.Schedule{
			Epoch: 0,
			StartTime: s.thisEpochStartTime,
			EndTime: s.thisEpochEndTime,
			WeightFee: decimal.NewFromFloat(s.thisEpochWeightFee),
			WeightMCB: decimal.NewFromFloat(s.thisEpochWeightMCB),
			WeightOI: decimal.NewFromFloat(s.thisEpochWeightOI),
		}).Error
	if err != nil {
		s.logger.Error("set default epoch error %s", err)
		panic(err)
	}
}


func (s *Syncer) Init() {
	s.blockSyncer.Init()
	// get this epoch number, thisEpochStartTime, thisEpochEndTime
	err := s.getEpoch()
	if err == NOT_IN_EPOCH {
		s.logger.Warn("warn %s", err)
	} else if err == EMPTY_SCHEDULE {
		s.logger.Warn("warn %s", err)
		s.defaultEpoch()
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
	for {
		select {
		case <-s.ctx.Done():
			ticker1min.Stop()
			s.logger.Info("Syncer receives shutdown signal.")
			return nil
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

func (s *Syncer) calculateOI(marginAccounts []*MarginAccount, blockNumber int64) decimal.Decimal {
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
	s.blockNumber, err = s.TimestampToBlockNumber(s.thisEpochStartTime)
	if err != nil {
		s.logger.Error("Failed to get timestampToBlockNumber %d", s.thisEpochStartTime)
		return
	}
	user, err := s.GetUsersBasedOnBlockNumber(s.blockNumber)
	for _, u := range user {
		oi := s.calculateOI(u.MarginAccounts, s.blockNumber)
		score := s.calScore(u.TotalFee, oi, u.StakedMCB)
		var userInfo = &mining.UserInfo{
			Trader:    u.ID,
			Fee:       u.TotalFee,  // TODO(ChampFu): decrease
			Stake:     u.StakedMCB, // TODO(chmpFU): calculate previous
			OI:        oi,          // TODO(chmpFU): calculate previous
			Score:     score,
			Timestamp: s.thisEpochStartTime,
			Epoch:     s.epoch,
		}
		s.db.Create(userInfo)
	}
	return

	// 	if len(response.Data.Users) == 500 {
	// 		// means there are more data to get
	// 		skip += 500
	// 		if skip == 5000 {
	// 			break
	// 		}
	// 	} else {
	// 		break
	// 	}
	// }
}

func (s *Syncer) syncState() {
	s.logger.Info("Sync state")
	var err error
	s.blockNumber, err = s.TimestampToBlockNumber(s.thisEpochStartTime)
	if err != nil {
		s.logger.Error("Failed to get timestampToBlockNumber %d", s.thisEpochStartTime)
		return
	}
	user, err := s.GetUsersBasedOnBlockNumber(s.blockNumber)
	for _, u := range user {
		oi := s.calculateOI(u.MarginAccounts, s.blockNumber)
		score := s.calScore(u.TotalFee, oi, u.StakedMCB)
		s.logger.Debug("user %+v", u)
		var userInfo = &mining.UserInfo{
			Trader:    u.ID,
			Fee:       u.TotalFee,  // TODO(ChampFu): decrease
			Stake:     u.StakedMCB, // TODO(chmpFU): calculate previous
			OI:        oi,          // TODO(chmpFU): calculate previous
			Score:     score,
			Timestamp: s.thisEpochStartTime,
			Epoch:     s.epoch,
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

//func (s *Syncer) syncPosition(timestamp time.Time) {
//	var params struct {
//		Query string `json:"query"`
//	}
//	queryFormat := `{
//		marginAccounts(where: {position_gt: "0"} first: 500 skip: %d) {
//			id
//			user {
//				id
//			}
//			position
//			entryValue
//		}
//	}`
//	skip := 0
//	for {
//		params.Query = fmt.Sprintf(queryFormat, skip)
//		err, code, res := f.httpClient.Post(f.graphUrl, nil, params, nil)
//		if err != nil || code != 200 {
//			f.logger.Info("get fee error. err:%s, code:%d", err, code)
//			return
//		}
//
//		var response struct {
//			Data struct {
//				MarginAccounts []struct {
//					ID   string `json:"id"`
//					User struct {
//						ID string `json:"id"`
//					}
//					Position   decimal.Decimal `json:"position"`
//					EntryValue decimal.Decimal `json:"entryValue"`
//				} `json:"marginAccounts"`
//			} `json:"data"`
//		}
//
//		err = json.Unmarshal(res, &response)
//		if err != nil {
//			f.logger.Error("Unmarshal error. err:%s", err)
//			return
//		}
//
//		for _, account := range response.Data.MarginAccounts {
//			newPosition := &mining.Position{
//				PerpetualAdd: account.ID,
//				Trader:       account.User.ID,
//				Position:     account.Position,
//				Timestamp:    timestamp.Unix(),
//			}
//			f.db.Create(newPosition)
//		}
//		if len(response.Data.MarginAccounts) == 500 {
//			// means there are more data to get
//			skip += 500
//		} else {
//			// we have got all
//			break
//		}
//	}
//}

// func (s *Syncer) syncFee(timestamp time.Time) {
// 	var params struct {
// 		Query string `json:"query"`
// 	}
// 	queryFormat := `{
// 		users(first: 500 skip: %d) {
// 			id
// 			totalFee
// 		}
// 	}`
// 	skip := 0
// 	for {
// 		params.Query = fmt.Sprintf(queryFormat, skip)
// 		err, code, res := f.httpClient.Post(f.graphUrl, nil, params, nil)
// 		if err != nil || code != 200 {
// 			f.logger.Info("get fee error. err:%s, code:%d", err, code)
// 			return
// 		}
//
// 		var response struct {
// 			Data struct {
// 				Users []struct {
// 					ID       string          `json:"id"`
// 					TotalFee decimal.Decimal `json:"totalFee"`
// 				} `json:"users"`
// 			} `json:"data"`
// 		}
//
// 		err = json.Unmarshal(res, &response)
// 		if err != nil {
// 			f.logger.Error("Unmarshal error. err:%s", err)
// 			return
// 		}
//
// 		if len(response.Data.Users) == 500 {
// 			// means there are more data to get
// 			skip += 500
// 		} else {
// 			// we have got all
// 			break
// 		}
// 	}
// }

func (s *Syncer) getEpoch() error {
	// get epoch from schedule database.
	now := time.Now().Unix()
	var schedules []*mining.Schedule
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
			s.epoch = schedule.Epoch
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

// func (c *Calculator) updateInitFee() {
// 	// reset
// 	c.initFee = make(map[string]decimal.Decimal)
// 	if c.epoch <= 0 {
// 		// there is no previous epoch
// 		return
// 	}
//
// 	var countTraders []struct {
// 		Trader string
// 	}
// 	// 1. get count traders first
// 	var count int
// 	err := c.db.Model(&mining.UserInfo{}).Select("DISTINCT trader").Where("epoch = ?", c.epoch-1).Scan(&countTraders).Error
// 	if err != nil {
// 		c.logger.Error("failed to get distinct count trader %s", err)
// 	} else {
// 		count = len(countTraders)
// 		c.logger.Info("there are %d distinct trader on previous epoch", count)
// 	}
// 	if count == 0 {
// 		// there are no trader on previous epoch
// 		return
// 	}
//
// 	// 2. get last fee of previous epoch
// 	var Fees []struct {
// 		Trader string
// 		Fee    decimal.Decimal
// 	}
// 	err = c.db.Model(&mining.UserInfo{}).Limit(count).Order("timestamp desc").Select("trader, fee").Where("epoch = ?", c.epoch-1).Scan(&Fees).Error
// 	for _, f := range Fees {
// 		c.initFee[f.Trader] = f.Fee
// 	}
// }

// func (c *Calculator) calculate(now time.Time) {
// 	if c.startTime.After(now) {
// 		// not yet
// 		return
// 	}
// 	nowDecimal := decimal.NewFromInt(now.Unix())
// 	lastTimestampDecimal := decimal.NewFromInt(c.lastTimestamp.Unix())
// 	minuteDecimal := decimal.NewFromInt(60)
//
// 	c.logger.Info("Calculation trading mining...")
// 	fromThisEpochStartTimeToNow := (nowDecimal.Add(c.thisEpochStartTimeDecimal.Neg())).Div(minuteDecimal)
// 	c.logger.Debug("fromThisEpochStartTimeToNow %s", fromThisEpochStartTimeToNow.String())
// 	if fromThisEpochStartTimeToNow.LessThanOrEqual(decimal.Zero) {
// 		c.logger.Error("this epoch start time decimal %s", c.thisEpochStartTimeDecimal.String())
// 		c.logger.Error("now %d", now.Unix())
// 		return
// 	}
// 	fromThisEpochStartTimeToLast := (lastTimestampDecimal.Add(c.thisEpochStartTimeDecimal.Neg())).Div(minuteDecimal)
// 	c.logger.Debug("fromThisEpochStartTimeToLast %s", fromThisEpochStartTimeToLast.String())
// 	if fromThisEpochStartTimeToLast.LessThanOrEqual(decimal.Zero) {
// 		c.logger.Warn("it will happen when first time doing calculation")
// 		c.logger.Warn("this epoch start time decimal %s", c.thisEpochStartTimeDecimal.String())
// 		c.logger.Warn("last %d", c.lastTimestamp.Unix())
// 	}
//
// 	var countTraders []struct {
// 		Trader string
// 	}
// 	var feeResults []struct {
// 		Trader    string
// 		Fee       decimal.Decimal
// 		Timestamp int64
// 	}
// 	var stakeResults []struct {
// 		Trader string
// 		Stake  decimal.Decimal
// 	}
// 	var positionResults []struct {
// 		Trader     string
// 		EntryValue decimal.Decimal
// 	}
// 	var userInfoResults []struct {
// 		Trader    string
// 		Fee       decimal.Decimal
// 		Stake     decimal.Decimal
// 		OI        decimal.Decimal
// 		Timestamp int64
// 	}
//
// 	err := c.db.Model(&mining.Fee{}).Select("DISTINCT trader").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&countTraders).Error
// 	if err != nil {
// 		c.logger.Error("failed to get fee %s", err)
// 		return
// 	}
// 	userCount := len(countTraders)
//
// 	// only get the latest one for all user {userCount}
// 	err = c.db.Model(&mining.Fee{}).Limit(userCount).Order("timestamp desc").Select("trader, fee, timestamp").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&feeResults).Error
// 	if err != nil {
// 		c.logger.Error("failed to get fee %s", err)
// 		return
// 	}
//
// 	for _, r := range feeResults {
// 		trader := r.Trader
// 		fee := r.Fee
// 		timestamp := r.Timestamp
// 		var stake decimal.Decimal
// 		var entryValue decimal.Decimal
// 		err = c.db.Model(&mining.Stake{}).Limit(1).Select("trader, AVG(stake) as stake").Where("trader = ? and timestamp > ?", trader, c.lastTimestamp.Unix()).Group("trader").Scan(&stakeResults).Error
// 		if err != nil {
// 			c.logger.Error("failed to get stake %s", err)
// 			return
// 		}
// 		if len(stakeResults) == 1 {
// 			stake = stakeResults[0].Stake
// 		} else if len(stakeResults) == 0 {
// 			// means this user don't have stake now.
// 			stake = decimal.Zero
// 		}
//
// 		err = c.db.Model(&mining.Position{}).Limit(1).Select("trader, AVG(entry_value) as entry_value").Where("trader = ? and timestamp > ?", trader, c.lastTimestamp.Unix()).Group("trader").Scan(&positionResults).Error
// 		if err != nil {
// 			c.logger.Error("failed to get position %s", err)
// 			return
// 		}
// 		if len(positionResults) == 1 {
// 			entryValue = positionResults[0].EntryValue
// 		} else if len(positionResults) == 0 {
// 			// means this user don't have position now.
// 			entryValue = decimal.Zero
// 		}
//
// 		// the fee is now_fee - start_fee.
// 		if iFee, match := c.initFee[r.Trader]; match {
// 			fee = fee.Add(iFee.Neg())
// 			if fee.LessThan(decimal.Zero) { // can be zero
// 				c.logger.Error("feeResult %+v", r)
// 				c.logger.Error("fee %s", fee.String())
// 				c.logger.Error("init fee %s", iFee.Neg().String())
// 				return
// 			}
// 		}
// 		// the stake is (stake * interval) + (pre_stake * (lastTimeStamp - start_time)) / now - start_time
// 		// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
// 		thisEntryValue := entryValue.Mul(c.intervalDecimal)
// 		thisStakeValue := stake.Mul(c.intervalDecimal)
// 		if thisStakeValue.LessThan(decimal.Zero) {
// 			c.logger.Error("thisStakeValue is less than zero")
// 			c.logger.Error("value %s", stake.String())
// 			return
// 		}
// 		if thisEntryValue.LessThan(decimal.Zero) {
// 			c.logger.Error("thisEntryValue is less than zero")
// 			c.logger.Error("value %s", entryValue.String())
// 			return
// 		}
//
// 		// get this epoch but latest info
// 		err = c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee, stake, oi").Where("trader = ? and epoch = ?", trader, c.epoch).Scan(&userInfoResults).Error
// 		if err != nil {
// 			c.logger.Error("failed to get user info %s", err)
// 			return
// 		}
// 		var finalOI decimal.Decimal
// 		var finalStake decimal.Decimal
// 		if len(userInfoResults) == 0 {
// 			// there is no previous info, means stake without pre_stake, oi without pre_oi
// 			finalOI = thisEntryValue.Div(fromThisEpochStartTimeToNow)
// 			finalStake = thisStakeValue.Div(fromThisEpochStartTimeToNow)
// 		} else {
// 			// the stake is (stake * interval) + (pre_stake * (lastTimeStamp - start_time)) / now - start_time
// 			// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
// 			pre := userInfoResults[0]
// 			preEntryValue := pre.OI.Mul(fromThisEpochStartTimeToLast)
// 			if preEntryValue.LessThan(decimal.Zero) {
// 				c.logger.Error("preEntry is less than zero")
// 				c.logger.Error("value %s", pre.OI.String())
// 				return
// 			}
// 			preStake := pre.Stake.Mul(fromThisEpochStartTimeToLast)
// 			if preStake.LessThan(decimal.Zero) {
// 				c.logger.Error("preStake is less than zero")
// 				c.logger.Error("value %s", pre.Stake.String())
// 				return
// 			}
// 			finalOI = (thisEntryValue.Add(preEntryValue)).Div(fromThisEpochStartTimeToNow)
// 			finalStake = (thisStakeValue.Add(preStake)).Div(fromThisEpochStartTimeToNow)
// 		}
// 		score := c.calScore(fee, finalOI, finalStake)
// 		c.db.Create(&mining.UserInfo{
// 			Trader:    trader,
// 			Fee:       fee,
// 			OI:        finalOI,
// 			Stake:     finalStake,
// 			Score:     score,
// 			Timestamp: timestamp,
// 			Epoch:     c.epoch,
// 		})
// 	}
// 	c.lastTimestamp = &now
// }
