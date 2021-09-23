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
	feeUrl     string
	oiUrl      string
	ctx        context.Context
	httpClient *utils.Client
	logger     logging.Logger
	checkpoint int64
	interval   time.Duration
	db         *gorm.DB
}

func NewSyncer(ctx context.Context, logger logging.Logger, feeUrl string, oiUrl string, interval int) (*Syncer, error) {
	syncer := &Syncer{
		ctx:        ctx,
		httpClient: utils.NewHttpClient(transport, logger),
		feeUrl:     feeUrl,
		oiUrl:      oiUrl,
		interval:   time.Duration(interval),
		db:         database.GetDB(),
	}
	return syncer, nil
}

func (f *Syncer) Run() error {
	for {
		select {
		case <-f.ctx.Done():
			return nil
		case <-time.After(f.interval * time.Second):
			now := time.Now()
			f.syncFee(now)
			f.syncOI(now)
			f.syncStack(now)
		}
	}
}

func (f *Syncer) syncOI(timestamp time.Time) {
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		marginAccounts(where: {position_gt: "0"}) {
			id
			user {
				id
			}
			position
			entryValue
		}
	}`
	params.Query = fmt.Sprintf(queryFormat)

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
		newOI := &mining.OpenInterest{
			PerpetualID: account.ID,
			User:        account.User.ID,
			Position:    account.Position,
			OI:          account.EntryValue, // TODO(ChampFu): OI = position * mark_price(from oracle)
			Timestamp: timestamp.Unix(),
		}
		f.db.Create(newOI)
	}
}

func (f *Syncer) syncStack(timestamp time.Time) {

}

func (f *Syncer) syncFee(timestamp time.Time) {
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		users {
			id
			totalFee
		}
	}`
	params.Query = fmt.Sprintf(queryFormat)

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
			User:      user.ID,
			Fee:       user.TotalFee,
			Timestamp: timestamp.Unix(),
		}
		f.db.Create(newFee)
	}

	for _, user := range response.Data.Users {
		newStack := &mining.Stack{
			User: user.ID,
			Stack: decimal.NewFromInt(100), // TODO(ChampFu)
			Timestamp: timestamp.Unix(),
		}
		f.db.Create(newStack)
	}
}
