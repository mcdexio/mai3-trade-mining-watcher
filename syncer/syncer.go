package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"net"
	"net/http"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
)

var transport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout: 500 * time.Millisecond,
	}).DialContext,
	TLSHandshakeTimeout: 1000 * time.Millisecond,
	MaxIdleConns:        100,
	IdleConnTimeout:     30 * time.Second,
}

type Syncer struct {
	feeUrl         string
	oiUrl          string
	stackUrl       string
	ctx            context.Context
	httpClient     *utils.Client
	logger         logging.Logger
	intervalSecond time.Duration
	startTime      *time.Time
	db             *gorm.DB
}

func NewSyncer(ctx context.Context, logger logging.Logger, feeUrl string, oiUrl string, stackUrl string, intervalSecond int, startTime *time.Time) (*Syncer, error) {
	syncer := &Syncer{
		ctx:            ctx,
		httpClient:     utils.NewHttpClient(transport, logger),
		logger:         logger,
		feeUrl:         feeUrl,
		oiUrl:          oiUrl,
		stackUrl:       stackUrl,
		intervalSecond: time.Duration(intervalSecond),
		startTime:      startTime,
		db:             database.GetDB(),
	}
	return syncer, nil
}

func (f *Syncer) Run() error {
	for {
		select {
		case <-f.ctx.Done():
			return nil
		case <-time.After(f.intervalSecond * time.Second):
			f.logger.Info("Sync Fee, Position, Stack")
			now := time.Now()
			if f.startTime.Before(now) {
				f.syncFee(now)
				f.syncPosition(now)
				f.syncStack(now)
			}
		}
	}
}

func (f *Syncer) syncPosition(timestamp time.Time) {
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		marginAccounts(where: {position_gt: "0"} first: 500 skip: %d) {
			id
			user {
				id
			}
			position
			entryValue
		}
	}`
	skip := 0
	for {
		params.Query = fmt.Sprintf(queryFormat, skip)
		err, code, res := f.httpClient.Post(f.oiUrl, nil, params, nil)
		if err != nil || code != 200 {
			f.logger.Info("get fee error. err:%s, code:%d", err, code)
			return
		}

		var response struct {
			Data struct {
				MarginAccounts []struct {
					ID   string `json:"id"`
					User struct {
						ID string `json:"id"`
					}
					Position   decimal.Decimal `json:"position"`
					EntryValue decimal.Decimal `json:"entryValue"`
				} `json:"marginAccounts"`
			} `json:"data"`
		}

		err = json.Unmarshal(res, &response)
		if err != nil {
			f.logger.Error("Unmarshal error. err:%s", err)
			return
		}

		for _, account := range response.Data.MarginAccounts {
			newPosition := &mining.Position{
				PerpetualAdd: account.ID,
				UserAdd:      account.User.ID,
				Position:     account.Position,
				EntryValue:   account.EntryValue, // TODO(ChampFu): OI = position * mark_price(from oracle)
				Timestamp:    timestamp.Unix(),
			}
			f.db.Create(newPosition)
		}
		if len(response.Data.MarginAccounts) == 500 {
			// means there are more data to get
			skip += 500
		} else {
			// we have got all
			break
		}
	}
}

func (f *Syncer) syncStack(timestamp time.Time) {

}

func (f *Syncer) syncFee(timestamp time.Time) {
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		users(first: 500 skip: %d) {
			id
			totalFee
		}
	}`
	skip := 0
	for {
		params.Query = fmt.Sprintf(queryFormat, skip)
		err, code, res := f.httpClient.Post(f.feeUrl, nil, params, nil)
		if err != nil || code != 200 {
			f.logger.Info("get fee error. err:%s, code:%d", err, code)
			return
		}

		var response struct {
			Data struct {
				Users []struct {
					ID       string          `json:"id"`
					TotalFee decimal.Decimal `json:"totalFee"`
				} `json:"users"`
			} `json:"data"`
		}

		err = json.Unmarshal(res, &response)
		if err != nil {
			f.logger.Error("Unmarshal error. err:%s", err)
			return
		}

		for _, user := range response.Data.Users {
			newFee := &mining.Fee{
				UserAdd:   user.ID,
				Fee:       user.TotalFee,
				Timestamp: timestamp.Unix(),
			}
			f.db.Create(newFee)
		}

		for _, user := range response.Data.Users {
			newStack := &mining.Stack{
				UserAdd:   user.ID,
				Stack:     decimal.NewFromInt(100), // TODO(ChampFu)
				Timestamp: timestamp.Unix(),
			}
			f.db.Create(newStack)
		}
		if len(response.Data.Users) == 500 {
			// means there are more data to get
			skip += 500
		} else {
			// we have got all
			break
		}
	}
}
