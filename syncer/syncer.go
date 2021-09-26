package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
)

var EPOCH_ERROR = errors.New("failed to get epoch from schedule db")

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
}

type User struct {
	ID             string          `json:"id"`
	StakedMCB      decimal.Decimal `json:"stakedMCB"`
	TotalFee       decimal.Decimal `json:"totalFee"`
	MarginAccounts []struct {
		ID       string          `json:"id"`
		Position decimal.Decimal `json:"position"`
	}
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

func (s Syncer) Init() {
	s.blockSyncer.Init()

	// get this epoch number, thisEpochStartTime, thisEpochEndTime
	err := s.getEpoch()
	if err == EPOCH_ERROR {
		s.logger.Warn("right now is not in this epoch")
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

func (s *Syncer) GetUsersBasedOnBlockNumber(blockNumber int64) ([]*User, error) {
	var retUser []*User
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
			retUser = append(retUser, &user)
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

func (s *Syncer) catchup() {
	var err error
	// depend on this epoch, catchup to now
	s.logger.Info("Catchup stats from %d until now", s.thisEpochStartTime)
	s.blockSyncer.catchup(s.thisEpochStartTime-60, time.Now().Unix())
	s.blockNumber, err = s.TimestampToBlockNumber(s.thisEpochStartTime)
	if err != nil {
		s.logger.Error("Failed to get timestampToBlockNumber %d", s.thisEpochStartTime)
		return
	}
	user, err := s.GetUsersBasedOnBlockNumber(s.blockNumber)
	for _, u := range user {
		for _, marginAccount := range u.MarginAccounts {
			marginAccount.ID
			s.GetMarkPriceBasedOnBlockNumber(s.blockNumber, .)
		}
	}
	//		var userInfo := &mining.UserInfo{
	//			Trader: user.ID,
	//			Fee: user.TotalFee,
	//			Stake: user.StakedMCB,
	//		}
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
}

func (s *Syncer) GetMarkPriceBasedOnBlockNumber(blockNumber int64, poolAddr string, perpetualIndex int) (*decimal.Decimal, error) {
	s.logger.Info("Get mark price based on block number %d", blockNumber)
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
	fmt.Println(response.Data.MarkPrices)
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

func (s *Syncer) syncStake(timestamp time.Time) {

}

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
	for _, schedule := range schedules {
		if now > schedule.StartTime && now < schedule.EndTime {
			s.epoch = schedule.Epoch
			s.thisEpochStartTime = schedule.StartTime
			s.thisEpochEndTime = schedule.EndTime
			return nil
		}
	}
	return EPOCH_ERROR
}

// func (s *Syncer) calScore() decimal.Decimal {
// 	// there are issue on decimal pow, so using float64
// 	feeInflate, _ := fee.Float64()
// 	oiInflate, _ := oi.Float64()
// 	stakeInflate, _ := stake.Float64()
// 	score := math.Pow(feeInflate, 0.7) + math.Pow(oiInflate, 0.3) + math.Pow(stakeInflate, 0.3)
// 	return decimal.NewFromFloat(score)
// }

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
