package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	mai3GraphUrl  string
	blockGraphUrl string

	// weight
	thisEpoch          int64
	thisEpochStartTime int64
	thisEpochEndTime   int64
	thisEpochWeightOI  decimal.Decimal
	thisEpochWeightMCB decimal.Decimal
	thisEpochWeightFee decimal.Decimal
}

type MarginAccount struct {
	ID       string          `json:"id"`
	Position decimal.Decimal `json:"position"`
}

type User struct {
	ID             string          `json:"id"`
	StakedMCB      decimal.Decimal `json:"stakedMCB"`
	TotalFee       decimal.Decimal `json:"totalFee"`
	UnlockMCBTime  int64           `json:"unlockMCBTime"`
	MarginAccounts []*MarginAccount
}

type Block struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Timestamp string `json:"timestamp"`
}

func NewSyncer(
	ctx context.Context, logger logging.Logger, mai3GraphUrl string, blockGraphUrl string,
	) *Syncer {
	return &Syncer{
		ctx:           ctx,
		httpClient:    utils.NewHttpClient(Transport, logger),
		logger:        logger,
		mai3GraphUrl:  mai3GraphUrl,
		blockGraphUrl: blockGraphUrl,
		db:            database.GetDB(),
	}
}

func (s *Syncer) setDefaultEpoch() {
	s.thisEpoch = 0
	s.thisEpochStartTime = 1632798801 // time.Now().Unix()
	s.thisEpochEndTime = s.thisEpochStartTime + 60*60*24*14
	s.thisEpochWeightMCB = decimal.NewFromFloat(0.3)
	s.thisEpochWeightFee = decimal.NewFromFloat(0.7)
	s.thisEpochWeightOI = decimal.NewFromFloat(0.3)
	err := s.db.Create(
		mining.Schedule{
			Epoch:     s.thisEpoch,
			StartTime: s.thisEpochStartTime,
			EndTime:   s.thisEpochEndTime,
			WeightFee: s.thisEpochWeightFee,
			WeightMCB: s.thisEpochWeightMCB,
			WeightOI:  s.thisEpochWeightOI,
		}).Error
	if err != nil {
		s.logger.Error("set default epoch error %s", err)
		panic(err)
	}
}

func (s *Syncer) Init() {
	// get this epoch number, thisEpochStartTime, thisEpochEndTime
	err := s.getEpoch()
	if err != nil {
		if err == NOT_IN_EPOCH {
			s.logger.Warn("warn %s", err)
		} else if err == EMPTY_SCHEDULE {
			s.logger.Warn("warn %s", err)
			s.setDefaultEpoch()
		} else {
			panic(err)
		}
	}
	now := time.Now().Unix()
	s.logger.Debug("check epoch started: eta=%v, now=%v", s.thisEpochStartTime, now)
	if now > s.thisEpochStartTime {
		s.logger.Warn("wait for epoch (%v, %v) starts", s.thisEpoch, s.thisEpochStartTime)
		time.Sleep(time.Duration(now - s.thisEpochStartTime))
	}
	inv := 5 * time.Second
	for {
		err := s.initUserStates()
		if err != nil {
			s.logger.Warn("fail to initialize user state, retry in %v seconds %w", inv, err)
			// TODO: do somthing with inv, backoff or make it configurable
			time.Sleep(inv)
			continue
		}
		break
	}
}

func (s *Syncer) Run() error {
	t := time.NewTimer(0)
	for {
		select {
		case <-s.ctx.Done():
			t.Stop()
			s.logger.Info("Syncer receives shutdown signal.")
			return nil
		case <-t.C:
			for {
				next, err := s.syncState()
				if err != nil {
					s.logger.Warn("fail to sync state, retry in 5 seconds %s", err)
					t.Reset(5 * time.Second)
					break
				}
				now := norm(time.Now().Unix())
				if next > now {
					t.Reset(time.Duration(next-now) * time.Second)
					break
				}
				t.Reset(100 * time.Millisecond)
			}
		}
	}
}

// getUserWithBlockNumberID try three times to get users depend on ID with order and filter
func (s *Syncer) getUserWithBlockNumberID(blockNumber int64, id string) ([]User, error) {
	// s.logger.Debug("Get users based on block number %d and order and filter by ID %s", blockNumber, id)
	query := `{
		users(first: 1000, block: {number: %d}, orderBy: id, orderDirection: asc,
			where: { id_gt: "%s" totalFee_gt: 0}
		) {
			id
			stakedMCB
			unlockMCBTime
			totalFee
			marginAccounts(where: { position_gt: 0}) {
				id
				position
			}
		}
	}`
	var response struct {
		Data struct {
			Users []User
		}
	}
	// try three times for each pagination.
	if err := s.queryGraph(s.mai3GraphUrl, &response, query, blockNumber, id); err != nil {
		return []User{}, errors.New("failed to get users in three times")
	}
	return response.Data.Users, nil
}

func (s *Syncer) GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error) {
	s.logger.Debug("Get users based on block number %d", blockNumber)
	var retUser []User

	idFilter := "0x0"
	for {
		users, err := s.getUserWithBlockNumberID(blockNumber, idFilter)
		if err != nil {
			return retUser, err
		}
		// success get user based on block number and idFilter
		retUser = append(retUser, users...)
		length := len(users)
		if length == 1000 {
			// means there are more users, update idFilter
			idFilter = users[length-1].ID
		} else {
			// means got all users
			return retUser, nil
		}
	}
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

func (s *Syncer) getSyncProgress() (int64, error) {
	var p mining.Progress
	err := s.db.Model(mining.Progress{}).Scan(&p).Where("table_name=user_info").Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.Warn("progress record not found, start from beginning")
			return s.thisEpochStartTime, nil
		}
		return 0, fmt.Errorf("fail to get progress: table=user_info %w", err)
	}
	if p.From == 0 {
		s.logger.Warn("found 0 progress, start from beginning")
		return s.thisEpochStartTime, nil
	}
	return p.From, nil
}

func (s *Syncer) setSyncProgress(ts int64) error {

	s.logger.Info("save ts: timestamp=%v", ts)

	p := &mining.Progress{TableName: "user_info", From: ts}
	if err := s.db.Save(p).Error; err != nil {
		return fmt.Errorf("fail to save progress: table=user_info, timestamp=%v %w", ts, err)
	}
	return nil
}

func (s *Syncer) initUserStates() error {
	s.logger.Debug("enter initUserStates")
	defer s.logger.Debug("leave initUserStates")
	p, err := s.getSyncProgress()
	if err != nil {
		return fmt.Errorf("fail to get sync progress %w", err)
	}
	if p > s.thisEpochStartTime {
		return nil
	}
	// init fee
	var users []User
	// TODO: query all total fee before p, if not exist, return

	// start tx
	for _, u := range users {
		r := &mining.UserInfo{
			Trader:    u.ID,
			InitFee:   u.TotalFee,
			Epoch:     s.thisEpoch,
			Timestamp: p,
		}
		err := s.db.Model(mining.UserInfo{}).Create(r).Error
		if err != nil {
			return fmt.Errorf("fail to create user info: record=%+v %w", r, err)
		}
	}
	if err := s.setSyncProgress(p); err != nil {
		return fmt.Errorf("fail to save sync progress %w", err)
	}
	// end tx
	return nil
}

// queryGraph return err if failed to get response from graph in three times
func (s *Syncer) queryGraph(graphUrl string, resp interface{}, query string, args ...interface{}) error {
	var params struct {
		Query string `json:"query"`
	}
	params.Query = fmt.Sprintf(query, args...)
	for i := 0; i < 3; i++ {
		err, code, res := s.httpClient.Post(graphUrl, nil, params, nil)
		if err != nil {
			s.logger.Error("fail to post http request %w", err)
			continue
		} else if code/100 != 2 {
			s.logger.Error("unexpected http response: %v", code)
			continue
		}
		err = json.Unmarshal(res, &resp)
		if err != nil {
			s.logger.Error("failed to unmarshal %w", err)
			continue
		}
		// success
		return nil
	}
	return errors.New("failed to get queryGraph in three times")
}

func norm(ts int64) int64 {
	return ts - ts%60
}

func normN(ts int64, inv int64) int64 {
	return ts - ts%inv
}

func (s *Syncer) syncState() (int64, error) {
	s.logger.Info("enter sync state")
	defer s.logger.Info("leave sync state")

	// begin tx
	p, err := s.getSyncProgress()
	if err != nil {
		return 0, fmt.Errorf("fail to get sync progress %w", err)
	}
	np := norm(p + 60)
	bn, err := s.TimestampToBlockNumber(np)
	if err != nil {
		return 0, fmt.Errorf("failed to get block number from timestamp: timestamp=%v %w", np, err)
	}
	users, err := s.GetUsersBasedOnBlockNumber(bn)
	if err != nil {
		return 0, fmt.Errorf("failed to get users on block number: blocknumber=%v %w", bn, err)
	}
	s.logger.Info("found %v users @%v", len(users), bn)
	// 1. preprocess
	{
		// acc_pos_value <= cur_pos_value
		err = s.db.Model(mining.UserInfo{}).
			Where("epoch=? and cur_pos_value <> 0", s.thisEpoch).
			UpdateColumn("acc_pos_value", gorm.Expr("acc_pos_value + cur_pos_value")).Error
		if err != nil {
			return 0, fmt.Errorf("failed to accumulate cur_post_value to acc_pos_value  %w", err)
		}
		// acc_stake_score <= cur_stake_score
		err = s.db.Model(mining.UserInfo{}).
			Where("epoch=? and cur_stake_score <> 0", s.thisEpoch).
			UpdateColumn("acc_stake_score", gorm.Expr("acc_stake_score + cur_stake_score")).Error
		if err != nil {
			return 0, fmt.Errorf("failed to accumulate cur_stake_score to acc_stake_score %w", err)
		}
		// cur_stake_score <= 0
		err = s.db.Model(mining.UserInfo{}).Where("epoch=?", s.thisEpoch).
			Updates(mining.UserInfo{CurPosValue: decimal.Zero, CurStakeScore: decimal.Zero, Timestamp: np}).Error
		if err != nil {
			return 0, fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
		}
	}
	// 2. update graph data
	prices, err := s.GetMarkPrices(bn)
	if err != nil {
		return 0, fmt.Errorf("fail to get mark prices %w", err)
	}
	uis := make([]*mining.UserInfo, len(users))
	for i, u := range users {
		pv, err := s.getPositionValue(u.MarginAccounts, bn, prices)
		if err != nil {
			return 0, fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
		}
		ss := s.getStakeScore(p, u.UnlockMCBTime, u.StakedMCB)
		ui := &mining.UserInfo{
			Trader:        u.ID,
			Epoch:         s.thisEpoch,
			CurPosValue:   pv,
			CurStakeScore: ss,
			AccFee:        u.TotalFee,
		}
		uis[i] = ui
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
		DoUpdates: clause.AssignmentColumns([]string{"cur_pos_value", "cur_stake_score", "acc_fee"}),
	}).Create(&uis).Error; err != nil {
		return 0, fmt.Errorf("failed to create user_info: size=%v %w", len(uis), err)
	}
	// 3. update score
	var (
		elapsed   = decimal.NewFromInt((p - s.thisEpochStartTime) / 60)
		remaining = decimal.NewFromInt((s.thisEpochEndTime - s.thisEpochStartTime) / 60).Sub(elapsed)
	)
	var all []*mining.UserInfo
	if err := s.db.Model(mining.UserInfo{}).Where("epoch=?", s.thisEpoch).Find(&all).Error; err != nil {
		return 0, fmt.Errorf("fail to fetch all users in this epoch %w", err)
	}
	for _, ui := range all {
		ui.Score = s.getScore(ui, elapsed, remaining)
	}
	if err := s.db.Save(&all).Error; err != nil {
		return 0, fmt.Errorf("failed to create user_info: size=%v %w", len(uis), err)
	}
	if err := s.setSyncProgress(np); err != nil {
		return 0, fmt.Errorf("fail to save sync progress %w", err)
	}
	// end tx

	return np, nil
}

func (s Syncer) getScore(ui *mining.UserInfo, elapsed decimal.Decimal, remain decimal.Decimal) decimal.Decimal {
	if ui.AccFee.IsZero() {
		return decimal.Zero
	}
	fee := ui.AccFee.Sub(ui.InitFee)
	if fee.IsZero() {
		return decimal.Zero
	}
	stake := ui.AccStakeScore.Add(ui.CurStakeScore.Mul(remain)).Div(elapsed.Add(remain))
	if stake.IsZero() {
		return decimal.Zero
	}
	posVal := ui.AccPosValue.Add(ui.CurPosValue.Mul(remain)).Div(elapsed.Add(remain))
	if posVal.IsZero() {
		return decimal.Zero
	}
	return fee.Pow(s.thisEpochWeightFee).
		Mul(stake.Pow(s.thisEpochWeightMCB)).
		Mul(posVal.Pow(s.thisEpochWeightOI))
}

func (s Syncer) getStakeScore(curTime int64, unlockTime int64, staked decimal.Decimal) decimal.Decimal {
	if unlockTime < curTime {
		return decimal.Zero
	}
	days := normN(unlockTime-curTime, 86400)
	return decimal.NewFromInt(days).Mul(staked)
}

func (s Syncer) getPositionValue(accounts []*MarginAccount, bn int64, cache map[string]*decimal.Decimal) (decimal.Decimal, error) {
	sum := decimal.Zero
	for _, a := range accounts {
		var price *decimal.Decimal
		// 0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0-0x00233150044aec4cba478d0bf0ecda0baaf5ad19
		perpId := strings.Join(strings.Split(a.ID, "-")[:2], "-")
		if v, ok := cache[perpId]; ok {
			price = v
		} else {
			addr, _, index, err := s.GetPoolAddrIndexUserID(a.ID)
			if err != nil {
				return sum, fmt.Errorf("fail to get pool address and index from id %w", err)
			}
			p, err := s.GetMarkPriceBasedOnBlockNumber(bn, addr, index)
			if err != nil {
				return sum, fmt.Errorf("fail to get mark price %w", err)
			}
			price = p
			cache[perpId] = p
		}
		sum = price.Mul(a.Position).Abs()
	}
	return sum, nil
}

func (s *Syncer) GetMarkPriceBasedOnBlockNumber(blockNumber int64, poolAddr string, perpetualIndex int) (*decimal.Decimal, error) {
	s.logger.Debug("Get mark price based on block number %d, poolAddr %s, perpetualIndex %d", blockNumber, poolAddr, perpetualIndex)
	query := `{
		markPrices(first: 1, block: { number: %d }, where: {id: "%s"}) {
    		id
    		price
    		timestamp
		}
	}`
	var resp struct {
		Data struct {
			MarkPrices []struct {
				ID    string          `json:"id"`
				Price decimal.Decimal `json:"price"`
			}
		}
	}
	id := fmt.Sprintf("%s-%d", poolAddr, perpetualIndex)
	if err := s.queryGraph(s.mai3GraphUrl, &resp, query, blockNumber, id); err != nil {
		return nil, fmt.Errorf("fail to get mark price %w", err)
	}
	if len(resp.Data.MarkPrices) == 0 {
		return nil, errors.New("empty mark price")
	}
	return &resp.Data.MarkPrices[0].Price, nil
}

func (s *Syncer) GetMarkPrices(bn int64) (map[string]*decimal.Decimal, error) {
	s.logger.Debug("Get mark price based on block number %d", bn)
	// TODO: 1000 may be not enough for all markets, need paging
	query := `{
		markPrices(first: 1000, block: { number: %v }) {
			id
			price
			timestamp
		}
	}`
	var resp struct {
		Data struct {
			MarkPrices []struct {
				ID    string           `json:"id"`
				Price *decimal.Decimal `json:"price"`
			}
		}
	}
	if err := s.queryGraph(s.mai3GraphUrl, &resp, query, bn); err != nil {
		return nil, fmt.Errorf("fail to get mark price %w", err)
	}
	prices := make(map[string]*decimal.Decimal)
	for _, p := range resp.Data.MarkPrices {
		prices[p.ID] = p.Price
	}
	return prices, nil
}

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
			s.thisEpoch = schedule.Epoch
			s.thisEpochStartTime = schedule.StartTime
			s.thisEpochEndTime = schedule.EndTime
			s.thisEpochWeightFee = schedule.WeightFee
			s.thisEpochWeightOI = schedule.WeightOI
			s.thisEpochWeightMCB = schedule.WeightMCB
			return nil
		}
	}
	return NOT_IN_EPOCH
}

// TimestampToBlockNumber which is the closest but less than or equal to timestamp
func (s *Syncer) TimestampToBlockNumber(timestamp int64) (int64, error) {
	s.logger.Debug("get block number which is the closest but less than or equal to timestamp %d", timestamp)
	query := `{
		blocks(
			first:1, orderBy: number, orderDirection: asc, 
			where: {timestamp_gt: %d}
		) {
			id
			number
			timestamp
		}
	}`
	var response struct {
		Data struct {
			Blocks []*Block
		}
	}
	// return err when can't get block number in three times
	if err := s.queryGraph(s.blockGraphUrl, &response, query, timestamp); err != nil {
		return -1, fmt.Errorf("fail to transform timestamp to block number in three times %w", err)
	}

	if len(response.Data.Blocks) != 1 {
		return -1, fmt.Errorf("length of block of response is not equal to 1: expect=1, actual=%v, timestamp=%v", len(response.Data.Blocks), timestamp)
	}
	number, err := strconv.Atoi(response.Data.Blocks[0].Number)
	if err != nil {
		return -1, fmt.Errorf("failed to convert block number from string to int err:%s", err)
	}
	return int64(number - 1), nil
}
