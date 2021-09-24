package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"net/http"
	"time"
)

type TMServer struct {
	ctx         context.Context
	logger      logging.Logger
	db          *gorm.DB
	mux         *http.ServeMux
	server      *http.Server
	nowEpoch    int
	intervalSec time.Duration
	score       map[int]decimal.Decimal
}

func NewTMServer(ctx context.Context, logger logging.Logger, intervalSec int) (*TMServer, error) {
	tmServer := &TMServer{
		logger:      logger,
		db:          database.GetDB(),
		ctx:         ctx,
		intervalSec: time.Duration(intervalSec),
	}
	return tmServer, nil
}

func (s *TMServer) Run() error {
	// get total status first
	s.getEpoch()
	s.calculateStatus()

	mux := http.NewServeMux()
	mux.HandleFunc("/tradingMining", s.OnQueryTradingMining)
	s.server = &http.Server{
		Addr:         ":9487",
		WriteTimeout: time.Second * 25,
		Handler:      mux,
	}

	s.logger.Info("Starting trading mining httpserver")
	err := s.server.ListenAndServe()
	if err != nil {
		if err == http.ErrServerClosed {
			s.logger.Critical("Server closed under request")
		} else {
			s.logger.Critical("Server closed unexpected", err)
		}
	}
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-time.After(s.intervalSec * time.Second):
			s.calculateStatus()
		}
	}
}

func (s *TMServer) getEpoch() {
	s.logger.Info("get epoch")
	var epochs []struct {
		Epoch int
	}
	// get the epoch
	err := s.db.Model(&mining.UserInfo{}).Limit(1).Order("epoch desc").Select("epoch").Scan(&epochs).Error
	if err != nil {
		s.logger.Error("failed to get user info %s", err)
	}
	if len(epochs) == 0 {
		// there is no epoch
		s.nowEpoch = 0
	} else {
		s.nowEpoch = epochs[0].Epoch
	}
	s.logger.Info("Epoch %d", s.nowEpoch)
}

func (s *TMServer) calculateStatus() {
	var startEpoch int
	if len(s.score) == 0 {
		// first time start this server
		s.score = make(map[int]decimal.Decimal)
		// sync from epoch 0
		startEpoch = 0
	} else {
		// only sync from this epoch
		startEpoch = s.nowEpoch
		s.score[s.nowEpoch] = decimal.Zero
	}

	s.logger.Info("calculate total status")
	for i := startEpoch; i <= s.nowEpoch; i++ {
		var countsTrader []struct {
			Trader string
		}
		var traders []struct {
			Trader string
			Score  decimal.Decimal
			Epoch  int
		}
		// get distinct count
		err := s.db.Model(&mining.UserInfo{}).Select("DISTINCT trader").Where("epoch = ?", i).Scan(&countsTrader).Error
		if err != nil {
			s.logger.Error("failed to get value from user info table err=%w", err)
		}
		count := len(countsTrader)
		if count == 0 {
			s.logger.Warn("there are no trader in this epoch %d", i)
			s.score[i] = decimal.Zero
			return
		} else {
			s.logger.Info("there are %d trader in this epoch %d", count, i)
		}
		err = s.db.Model(&mining.UserInfo{}).Limit(count).Select("trader, score").Order("timestamp desc").Where("epoch = ?", s.nowEpoch).Scan(&traders).Error
		if err != nil {
			s.logger.Error("failed to get value from user info table err=%w", err)
		}
		for _, t := range traders {
			s.score[i] = s.score[i].Add(t.Score)
		}
		s.logger.Info("this epoch %d total score %s", i, s.score[i])
	}
}

type EpochTradingMiningResp struct {
	Fee        string `json:"fee"`
	OI         string `json:"oi"`
	Stake      string `json:"stake"`
	Score      string `json:"score"`
	Proportion string `json:"proportion"`
}

func (s *TMServer) OnQueryTradingMining(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			_, ok := r.(error)
			if !ok {
				err := fmt.Errorf("%v", r)
				s.logger.Error("recover err:%s", err)
				http.Error(w, "internal error.", 400)
				return
			}
		}
	}()

	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")

	// request
	query := r.URL.Query()
	trader := query["trader"]
	if len(trader) == 0 {
		s.logger.Info("parameter invalid:%#v", query)
		http.Error(w, "empty parameter.", 400)
		return
	}
	queryTradingMiningResp := make(map[int]*EpochTradingMiningResp)
	for i := 0; i <= s.nowEpoch; i++ {
		rsp := mining.UserInfo{}
		err := s.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select(
			"score, fee, oi, stake, timestamp").Where("trader = ? and epoch = ?", trader[0], i).Scan(&rsp).Error
		if err != nil {
			s.logger.Error("failed to get value from user info table err=%w", err)
		}
		s.logger.Info("%+v", rsp)
		s.logger.Debug("score %+v", s.score)
		totalScore, match := s.score[i]
		if !match {
			s.logger.Error("failed to get total score %+v", s.score)
		}
		proportion := (rsp.Score.Div(totalScore)).String()
		resp := EpochTradingMiningResp{
			Fee:        rsp.Fee.String(),
			OI:         rsp.OI.String(),
			Stake:      rsp.Stake.String(),
			Score:      rsp.Score.String(),
			Proportion: proportion,
		}
		queryTradingMiningResp[i] = &resp
	}

	s.logger.Info("%+v", queryTradingMiningResp)
	json.NewEncoder(w).Encode(queryTradingMiningResp)
}
