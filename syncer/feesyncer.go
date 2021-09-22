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

type FeeSyncer struct {
	url        string
	ctx        context.Context
	httpClient *utils.Client
	logger     logging.Logger
	checkpoint int64
	interval   time.Duration
	db         *gorm.DB
}

func NewFeeSyncer(ctx context.Context, logger logging.Logger, url string, interval int) (*FeeSyncer, error) {
	FeeSyncer := &FeeSyncer{
		ctx:        ctx,
		httpClient: utils.NewHttpClient(transport, logger),
		url:        url,
		interval:   time.Duration(interval),
		db:         database.GetDB(),
	}
	return FeeSyncer, nil
}

func (f *FeeSyncer) Run() error {
	for {
		select {
		case <-f.ctx.Done():
			return nil
		case <-time.After(f.interval * time.Second):
			f.syncFee()
		}
	}
}

func (f *FeeSyncer) syncFee() {
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		users(first:5) {
			id
			totalFee
		}
	}`
	params.Query = fmt.Sprintf(queryFormat)

	err, code, res := f.httpClient.Post(f.url, nil, params, nil)
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
			Timestamp: time.Now().Unix(),
		}
		f.db.Create(newFee)
	}
}
