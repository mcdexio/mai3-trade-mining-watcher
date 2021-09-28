package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TMServer struct {
	ctx      context.Context
	logger   logging.Logger
	db       *gorm.DB
	mux      *http.ServeMux
	server   *http.Server
	nowEpoch int
	score    map[int]decimal.Decimal
}

type EpochTradingMiningResp struct {
	Fee        string `json:"fee"`
	OI         string `json:"oi"`
	Stake      string `json:"stake"`
	Score      string `json:"score"`
	Proportion string `json:"proportion"`
}

func NewTMServer(ctx context.Context, logger logging.Logger) *TMServer {
	tmServer := &TMServer{
		logger: logger,
		db:     database.GetDB(),
		ctx:    ctx,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/score", tmServer.OnQueryTradingMining)
	tmServer.server = &http.Server{
		Addr:         ":9487",
		WriteTimeout: time.Second * 25,
		Handler:      mux,
	}
	return tmServer
}

func (s *TMServer) Shutdown() error {
	return s.server.Shutdown(s.ctx)
}

func (s *TMServer) Run() error {
	s.logger.Info("Starting trading mining api httpserver")
	go func() {
		err := s.server.ListenAndServe()
		if err != nil {
			if err == http.ErrServerClosed {
				s.logger.Critical("Server closed under request")
			} else {
				s.logger.Critical("Server closed unexpected", err)
			}
		}
	}()

	for {
		if err := s.calculateTotalScore(); err == nil {
			break
		}
	}

	ticker1min := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-s.ctx.Done():
			ticker1min.Stop()
			s.logger.Info("Syncer receives shutdown signal.")
			return nil
		case <-ticker1min.C:
			for {
				if err := s.calculateTotalScore(); err == nil {
					break
				}
			}
		}
	}
}

func (s *TMServer) getEpoch() error {
	var epochs []struct {
		Epoch int
	}
	// get the epoch
	err := s.db.Model(&mining.UserInfo{}).Limit(1).Order("epoch desc").Select("epoch").Scan(&epochs).Error
	if err != nil {
		return fmt.Errorf("failed to get user info %s", err)
	}
	if len(epochs) == 0 {
		// there is no epoch
		s.nowEpoch = 0
	} else {
		s.nowEpoch = epochs[0].Epoch
	}
	s.logger.Debug("get epoch: %d", s.nowEpoch)
	return nil
}

func (s *TMServer) calculateTotalScore() error {
	for {
		if err := s.getEpoch(); err == nil {
			// success
			break
		}
	}

	var startEpoch int
	if len(s.score) == 0 {
		// first time start this server
		s.score = make(map[int]decimal.Decimal)
		// sync from epoch 0
		startEpoch = 0
	} else {
		// only sync from this epoch, because previous has been processed.
		startEpoch = s.nowEpoch
		s.score[s.nowEpoch] = decimal.Zero
	}

	s.logger.Info("calculate total status")
	for i := startEpoch; i <= s.nowEpoch; i++ {
		var traders []struct {
			Trader string
			Score  decimal.Decimal
			Epoch  int
		}
		err := s.db.Model(&mining.UserInfo{}).Select("score").Where("epoch = ?", i).Scan(&traders).Error
		if err != nil {
			return err
		}

		count := len(traders)
		s.logger.Info("there are %d trader in this epoch %d", count, i)
		if count == 0 {
			s.score[i] = decimal.Zero
			continue
		}

		for _, t := range traders {
			s.score[i] = s.score[i].Add(t.Score)
		}
		s.logger.Info("this epoch %d total score %s", i, s.score[i])
	}
	return nil
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
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")

	// request
	query := r.URL.Query()
	trader := query["trader"]
	if len(trader) == 0 || trader[0] == "" {
		s.logger.Info("empty parameter:%#v", query)
		s.jsonError(w, "empty parameter", 400)
		return
	}
	queryTradingMiningResp := make(map[int]*EpochTradingMiningResp)
	for i := 0; i <= s.nowEpoch; i++ {
		rsp := mining.UserInfo{}
		err := s.db.Model(&mining.UserInfo{}).Limit(1).Select(
			"acc_fee, init_fee, acc_pos_value, cur_pos_value, acc_stake_score, cur_stake_score, score").Where("trader = ? and epoch = ?", trader[0], i).Scan(&rsp).Error
		if err != nil {
			s.logger.Error("failed to get value from user info table err=%w", err)
			s.jsonError(w, "internal error", 400)
			return
		}
		s.logger.Info("user info %+v of epoch %d", rsp, i)
		s.logger.Debug("score %+v", s.score)
		totalScore, match := s.score[i]
		if !match {
			s.logger.Error("failed to get total score %+v", s.score)
			s.jsonError(w, "internal error", 400)
			return
		}
		var proportion string
		if totalScore.IsZero() {
			proportion = "0"
		} else {
			proportion = (rsp.Score.Div(totalScore)).String()
		}
		resp := EpochTradingMiningResp{
			Fee:        rsp.AccFee.Sub(rsp.InitFee).String(),
			OI:         rsp.AccPosValue.Add(rsp.CurPosValue).String(),
			Stake:      rsp.AccStakeScore.Add(rsp.CurStakeScore).String(),
			Score:      rsp.Score.String(),
			Proportion: proportion,
		}
		queryTradingMiningResp[i] = &resp
	}

	s.logger.Info("%+v", queryTradingMiningResp)
	json.NewEncoder(w).Encode(queryTradingMiningResp)
}

func (s *TMServer) jsonError(w http.ResponseWriter, err interface{}, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	var msg struct {
		Error string `json:"error"`
	}
	msg.Error = err.(string)
	json.NewEncoder(w).Encode(msg)
}
