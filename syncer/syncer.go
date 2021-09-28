package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
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

var (
	PROGRESS_SYNC_STATE = "user_info" // compatible
	PROGRESS_INIT_FEE   = "user_info.init_fee"
)

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
	curEpochConfig *mining.Schedule

	// default if you don't set epoch in schedule database
	defaultEpochStartTime int64
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

type MarkPrice struct {
	ID    string           `json:"id"`
	Price *decimal.Decimal `json:"price"`
}

func NewSyncer(
	ctx context.Context, logger logging.Logger, mai3GraphUrl string, blockGraphUrl string,
	defaultEpochStartTime int64) *Syncer {
	return &Syncer{
		ctx:                   ctx,
		httpClient:            utils.NewHttpClient(Transport, logger),
		logger:                logger,
		mai3GraphUrl:          mai3GraphUrl,
		blockGraphUrl:         blockGraphUrl,
		db:                    database.GetDB(),
		defaultEpochStartTime: defaultEpochStartTime,
	}
}

func (s *Syncer) setDefaultEpoch() int64 {
	// start := time.Now().Unix()
	start := s.defaultEpochStartTime
	err := s.db.Create(&mining.Schedule{
		Epoch:     0,
		StartTime: start,
		EndTime:   start + 60*60*24*14,
		WeightFee: decimal.NewFromFloat(0.7),
		WeightMCB: decimal.NewFromFloat(0.3),
		WeightOI:  decimal.NewFromFloat(0.3),
	}).Error
	if err != nil {
		s.logger.Error("set default epoch error %s", err)
		panic(err)
	}

	return start
}

func (s *Syncer) Run() error {
	for {
		if err := s.run(s.ctx); err != nil {
			if !errors.Is(err, NOT_IN_EPOCH) {
				s.logger.Warn("error occurs while running: %w", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		s.logger.Warn("not in any epoch")
		time.Sleep(5 * time.Second)
	}
}

func (s *Syncer) run(ctx context.Context) error {
	// try to find out last epoch
	cp, err := s.lastProgress(PROGRESS_SYNC_STATE)
	if err != nil {
		return err
	}
	// brand new start, no last progress
	if cp == 0 {
		// on very init state, here the cp should be 0
		// which means it is impossible to detect which epoch we are in
		// so set default epoch information from bin/config DEFAULT_EPOCH_0_START_TIME
		cp = s.setDefaultEpoch()
	}
	e, err := s.detectEpoch(cp)
	if err != nil {
		return err
	}
	s.logger.Info("found in epoch %+v", e)
	// set epoch
	s.curEpochConfig = e
	// set init fee
	if err := s.initUserStates(); err != nil {
		return fmt.Errorf("fail to init user states: %w", err)
	}
	// sync state
	np := cp
	for np < s.curEpochConfig.EndTime {
		select {
		case <-ctx.Done():
			return nil
		default:
			p, err := s.syncState()
			if err != nil {
				s.logger.Warn("fail to sync state, retry in 5 seconds %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
			np = p + 60
			now := norm(time.Now().Unix())
			if np > now && np < s.curEpochConfig.EndTime {
				// sleep until next tick
				time.Sleep(time.Duration(np-now) * time.Second)
			}
		}
	}
	s.logger.Info("epoch done: epoch=%+v", s.curEpochConfig)
	return nil
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

func (s *Syncer) lastProgress(name string) (int64, error) {
	var p mining.Progress
	err := s.db.Model(mining.Progress{}).Where("table_name=?", name).Order("epoch desc").First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get last progress: table=%s %w", name, err)
	}
	return p.From, nil
}

func (s *Syncer) getProgress(name string, epoch int64) (int64, error) {
	var p mining.Progress
	err := s.db.Model(mining.Progress{}).Where("table_name=? and epoch=?", name, epoch).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get progress: table=%s %w", name, err)
	}
	return p.From, nil
}

func (s *Syncer) setProgress(name string, ts int64, epoch int64) error {
	s.logger.Info("save progress for %v: timestamp=%v", name, ts)
	p := &mining.Progress{TableName: types.TableName(name), From: ts, Epoch: epoch}
	if err := s.db.Save(p).Error; err != nil {
		return fmt.Errorf("fail to save progress: table=%v, timestamp=%v %w", name, ts, err)
	}
	return nil
}

func (s *Syncer) initUserStates() error {
	s.logger.Debug("enter initUserStates")
	defer s.logger.Debug("leave initUserStates")
	p, err := s.getProgress(PROGRESS_INIT_FEE, s.curEpochConfig.Epoch)
	if err != nil {
		return fmt.Errorf("fail to get sync progress %w", err)
	}
	// already synced
	if p != 0 && p == s.curEpochConfig.StartTime {
		s.logger.Info("fee already initialied")
		return nil
	}
	// query all total fee before this epoch start time, if not exist, return
	startBn, err := s.TimestampToBlockNumber(s.curEpochConfig.StartTime)
	if err != nil {
		return err
	}
	users, err := s.GetUsersBasedOnBlockNumber(startBn)
	if err != nil {
		return err
	}
	uis := make([]*mining.UserInfo, len(users))
	for i, u := range users {
		uis[i] = &mining.UserInfo{
			Trader:  strings.ToLower(u.ID),
			Epoch:   s.curEpochConfig.Epoch,
			InitFee: u.TotalFee,
		}
	}
	err = db.WithTransaction(s.db, func(tx *gorm.DB) error {
		cp, err := s.getProgress(PROGRESS_INIT_FEE, s.curEpochConfig.Epoch)
		if err != nil {
			return fmt.Errorf("fail to get sync progress %w", err)
		}
		// safe guard
		if cp != p {
			return fmt.Errorf("progress changed, somewhere may run another instance")
		}
		if err := s.db.Model(mining.UserInfo{}).Create(&uis).Error; err != nil {
			return fmt.Errorf("fail to create init user info %w", err)
		}
		if err := s.setProgress(PROGRESS_INIT_FEE, s.curEpochConfig.StartTime, s.curEpochConfig.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("fail to init fee of all users for new epoch: epoch=%v %w", s.curEpochConfig.Epoch, err)
	}
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

func (s *Syncer) syncState() (int64, error) {
	s.logger.Info("enter sync state")
	defer s.logger.Info("leave sync state")
	p, err := s.getProgress(PROGRESS_SYNC_STATE, s.curEpochConfig.Epoch)
	if err != nil {
		return 0, fmt.Errorf("fail to get sync progress %w", err)
	}
	var (
		lp int64
		np int64
	)
	if p == 0 {
		lp = s.curEpochConfig.StartTime
	} else {
		lp = p
	}
	np = norm(lp + 60)
	bn, err := s.TimestampToBlockNumber(np)
	if err != nil {
		return 0, fmt.Errorf("failed to get block number from timestamp: timestamp=%v %w", np, err)
	}
	users, err := s.GetUsersBasedOnBlockNumber(bn)
	if err != nil {
		return 0, fmt.Errorf("failed to get users on block number: blocknumber=%v %w", bn, err)
	}
	s.logger.Info("found %v users @%v", len(users), bn)
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
		// ss is (unlock time - now) * u.StackedMCB <=> s = n * t
		ss := s.getStakeScore(np, u.UnlockMCBTime, u.StakedMCB)
		ui := &mining.UserInfo{
			Trader:        strings.ToLower(u.ID),
			Epoch:         s.curEpochConfig.Epoch,
			CurPosValue:   pv,
			CurStakeScore: ss,
			AccFee:        u.TotalFee,
		}
		uis[i] = ui
	}
	// begin tx
	err = db.WithTransaction(s.db, func(tx *gorm.DB) error {
		curP, err := s.getProgress(PROGRESS_SYNC_STATE, s.curEpochConfig.Epoch)
		if err != nil {
			return fmt.Errorf("fail to get sync progress %w", err)
		}
		if curP != p {
			return fmt.Errorf("progress changed, somewhere may run another instance")
		}
		// acc_pos_value += cur_pos_value if cur_pos_value != 0
		err = s.db.Model(mining.UserInfo{}).
			Where("epoch=? and cur_pos_value <> 0", s.curEpochConfig.Epoch).
			UpdateColumn("acc_pos_value", gorm.Expr("acc_pos_value + cur_pos_value")).Error
		if err != nil {
			return fmt.Errorf("failed to accumulate cur_post_value to acc_pos_value  %w", err)
		}
		// acc_stake_score += cur_stake_score if cur_stake_score != 0
		err = s.db.Model(mining.UserInfo{}).
			Where("epoch=? and cur_stake_score <> 0", s.curEpochConfig.Epoch).
			UpdateColumn("acc_stake_score", gorm.Expr("acc_stake_score + cur_stake_score")).Error
		if err != nil {
			return fmt.Errorf("failed to accumulate cur_stake_score to acc_stake_score %w", err)
		}
		// cur_stake_score <= 0 and cur_pos_value <= 0
		err = s.db.Model(mining.UserInfo{}).Where("epoch=?", s.curEpochConfig.Epoch).
			Updates(mining.UserInfo{CurPosValue: decimal.Zero, CurStakeScore: decimal.Zero, Timestamp: np}).Error
		if err != nil {
			return fmt.Errorf("failed to set cur_stake_score and cur_pos_value to 0 %w", err)
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "trader"}, {Name: "epoch"}},
			DoUpdates: clause.AssignmentColumns([]string{"cur_pos_value", "cur_stake_score", "acc_fee"}),
		}).Create(&uis).Error; err != nil {
			return fmt.Errorf("failed to create user_info: size=%v %w", len(uis), err)
		}
		// 3. update score
		var (
			elapsed = decimal.NewFromInt((p - s.curEpochConfig.StartTime) / 60) // Minutes
		)
		var all []*mining.UserInfo
		if err := s.db.Model(mining.UserInfo{}).Where("epoch=?", s.curEpochConfig.Epoch).Find(&all).Error; err != nil {
			return fmt.Errorf("fail to fetch all users in this epoch %w", err)
		}
		for _, ui := range all {
			ui.Score = s.getScore(ui, elapsed)
		}
		if err := s.db.Save(&all).Error; err != nil {
			return fmt.Errorf("failed to create user_info: size=%v %w", len(uis), err)
		}
		if err := s.setProgress(PROGRESS_SYNC_STATE, np, s.curEpochConfig.Epoch); err != nil {
			return fmt.Errorf("fail to save sync progress %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("fail to update state to db %w", err)
	}
	return np, nil
}

func (s Syncer) getStakeScore(curTime int64, unlockTime int64, staked decimal.Decimal) decimal.Decimal {
	if unlockTime < curTime {
		return decimal.Zero
	}
	// floor to 1 if less than 1 day
	days := int64(math.Ceil(float64(unlockTime-curTime) / 86400))
	return decimal.NewFromInt(days).Mul(staked)
}

func (s Syncer) getScore(ui *mining.UserInfo, elapsed decimal.Decimal) decimal.Decimal {
	if ui.AccFee.IsZero() {
		return decimal.Zero
	}
	fee := ui.AccFee.Sub(ui.InitFee)
	if fee.IsZero() {
		return decimal.Zero
	}
	stake := ui.AccStakeScore.Add(ui.CurStakeScore).Div(elapsed)
	if stake.IsZero() {
		return decimal.Zero
	}
	posVal := ui.AccPosValue.Add(ui.CurPosValue).Div(elapsed)
	if posVal.IsZero() {
		return decimal.Zero
	}
	return fee.Pow(s.curEpochConfig.WeightFee).
		Mul(stake.Pow(s.curEpochConfig.WeightMCB)).
		Mul(posVal.Pow(s.curEpochConfig.WeightOI))
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

// getMarkPricesWithBlockNumberID try three times to get markPrices depend on ID with order and filter
func (s *Syncer) getMarkPricesWithBlockNumberID(blockNumber int64, id string) ([]MarkPrice, error) {
	s.logger.Debug("Get mark price based on block number %d and order and filter by ID %s", blockNumber, id)
	query := `{
		markPrices(first: 1000, block: { number: %v }, orderBy: id, orderDirection: asc,
			where: { id_gt: "%s" }
		) {
			id
			price
			timestamp
		}
	}`
	var resp struct {
		Data struct {
			MarkPrices []MarkPrice
		}
	}
	if err := s.queryGraph(s.mai3GraphUrl, &resp, query, blockNumber, id); err != nil {
		return nil, fmt.Errorf("fail to get mark price %w", err)
	}
	return resp.Data.MarkPrices, nil
}

func (s *Syncer) GetMarkPrices(bn int64) (map[string]*decimal.Decimal, error) {
	s.logger.Debug("Get mark price based on block number %d", bn)
	prices := make(map[string]*decimal.Decimal)
	idFilter := "0x0"
	for {
		markPrices, err := s.getMarkPricesWithBlockNumberID(bn, idFilter)
		if err != nil {
			return prices, nil
		}
		// success get mark prices on block number and idFilter
		for _, p := range markPrices {
			prices[p.ID] = p.Price
		}
		length := len(markPrices)
		if length == 1000 {
			// means there are more markPrices, update idFilter
			idFilter = markPrices[length-1].ID
		} else {
			// means got all markPrices
			return prices, nil
		}
	}
}

func (s *Syncer) detectEpoch(p int64) (*mining.Schedule, error) {
	// get epoch from schedule database.
	// detect which epoch is time(p) in.
	var ss []*mining.Schedule
	// TODO(champFu): check with @yz if there are two epoch 0 and 1, endTime of these two epochs are all bigger than p+60, we get 0, maybe check len(ss) == 1
	if err := s.db.Model(&mining.Schedule{}).Where("end_time>?", p+60).Order("epoch asc").Find(&ss).Error; err != nil {
		return nil, fmt.Errorf("fail to found epoch config %w", err)
	}
	if len(ss) == 0 {
		// not in any epoch
		return nil, NOT_IN_EPOCH
	}
	return ss[0], nil
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
