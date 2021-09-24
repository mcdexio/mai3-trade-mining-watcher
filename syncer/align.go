package syncer

import (
	"context"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"gorm.io/gorm"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
)

type Aligner struct {
	feeUrl         string
	oiUrl          string
	stakeUrl       string
	ctx            context.Context
	httpClient     *utils.Client
	logger         logging.Logger
	intervalSecond time.Duration
	db             *gorm.DB
}

func NewAligner(
	ctx context.Context,
	logger logging.Logger,
	feeUrl string,
	oiUrl string,
	stakeUrl string,
	intervalSecond int,
) *Aligner {
	aligner := &Aligner{
		ctx:            ctx,
		httpClient:     utils.NewHttpClient(transport, logger),
		logger:         logger,
		feeUrl:         feeUrl,
		oiUrl:          oiUrl,
		stakeUrl:       stakeUrl,
		intervalSecond: time.Duration(intervalSecond),
		db:             database.GetDB(),
	}
	return aligner
}

// func (a *Aligner) Run() error {
// 	for {
// 		select {
// 		case <-f.ctx.Done():
// 			return nil
// 		case <-time.After()
//
// 		}
// 	}
// }
//
