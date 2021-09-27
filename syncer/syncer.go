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
	ctx context.Context, logger logging.Logger, mai3GraphUrl string, blockGraphUrl string
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
	if err == NOT_IN_EPOCH {
		s.logger.Warn("warn %s", err)
	} else if err == EMPTY_SCHEDULE {
		s.logger.Warn("warn %s", err)
		s.setDefaultEpoch()
	} else if err != nil {
		s.logger.Error("error %s", err)
		panic(err)
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

func (s *Syncer) GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error) {
	var users []User
	s.logger.Info("get users based on block number %d", blockNumber)
	query := `{
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
		var resp struct {
			Data struct {
				Users []User
			}
		}
		if err := s.queryGraph(&resp, query, skip, blockNumber); err != nil {
			return nil, fmt.Errorf("fail to get users on block: skip=%v, block=%v %w", skip, blockNumber, err)
		}
		users = append(users, resp.Data.Users...)
		if len(resp.Data.Users) == 500 {
			// means there are more data to get
			skip += 500
			if skip == 5000 {
				return nil, fmt.Errorf("user more than 5000, but we don't have filter")
			}
		} else {
			break
		}
	}
	return users, nil
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
		s.logger.Warn("found 0 progress, start from 1632798801")
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