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
	"gorm.io/gorm/clause"
	"net/http"
	"strconv"
	"time"
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
	mux.HandleFunc("/tradingMining", tmServer.OnQueryTradingMining)
	mux.HandleFunc("/healthCheckup", tmServer.OnQueryHealthCheckup)
	mux.HandleFunc("/setEpoch", tmServer.OnQuerySetEpoch)
	tmServer.server = &http.Server{
		Addr:         ":9487",
		WriteTimeout: time.Second * 25,
		Handler:      mux,
	}
	return tmServer
}

func (s *TMServer) OnQueryHealthCheckup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	resp := make(map[string]string)
	resp["message"] = "alive"
	json.NewEncoder(w).Encode(resp)
}

func (s *TMServer) Shutdown() error {
	return s.server.Shutdown(s.ctx)
}

func (s *TMServer) Run() error {
	s.logger.Info("Starting trading mining httpserver")
	s.getEpoch()
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

	s.calculateStatus()
	ticker1min := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-s.ctx.Done():
			ticker1min.Stop()
			s.logger.Info("Syncer receives shutdown signal.")
			return nil
		case <-ticker1min.C:
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
		s.logger.Error("Failed to get user info %s", err)
		return
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
	s.getEpoch()
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
			return
		}
		count := len(countsTrader)
		if count == 0 {
			s.logger.Warn("there are no trader in this epoch %d", i)
			s.score[i] = decimal.Zero
			return
		} else {
			s.logger.Info("there are %d trader in this epoch %d", count, i)
		}
		err = s.db.Model(&mining.UserInfo{}).Limit(count).Select("trader, score").Order("timestamp desc").Where("epoch = ?", i).Scan(&traders).Error
		if err != nil {
			s.logger.Error("failed to get value from user info table err=%w", err)
		}
		for _, t := range traders {
			s.score[i] = s.score[i].Add(t.Score)
		}
		s.logger.Info("this epoch %d total score %s", i, s.score[i])
	}
}

func (s *TMServer) OnQuerySetEpoch(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			_, ok := r.(error)
			if !ok {
				err := fmt.Errorf("%v", r)
				s.logger.Error("recover err:%s", err)
				s.jsonError(w, "internal error.", 400)
				return
			}
		}
	}()

	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")

	query := r.URL.Query()
	epoch := query["epoch"]
	startTime := query["startTime"]
	endTime := query["endTime"]
	weightFee := query["weightFee"]
	weightOI := query["weightOI"]
	weightMCB := query["weightMCB"]
	if (len(epoch) == 0 || epoch[0] == "") ||
		(len(startTime) == 0 || startTime[0] == "") ||
		(len(endTime) == 0 || endTime[0] == "") ||
		(len(weightOI) == 0 || weightOI[0] == "") ||
		(len(weightFee) == 0 || weightFee[0] == "") ||
		(len(weightMCB) == 0 || weightMCB[0] == "") {
		s.logger.Info("empty parameter:%#v", query)
		s.jsonError(w, "empty parameter", 400)
		return
	}
	s.logger.Debug(
		"epoch %s, startTime %s, endTime %s, weightFee %s, weightOI %s, weightMCB %s",
		epoch, startTime, endTime, weightFee, weightOI, weightMCB,
	)

	e, err := strconv.Atoi(epoch[0])
	if err != nil {
		s.logger.Info("parameter invalid:%#v", query)
		s.jsonError(w, "parameter invalid", 400)
		return
	}
	st, err := strconv.Atoi(startTime[0])
	if err != nil {
		s.logger.Info("parameter invalid:%#v", query)
		s.jsonError(w, "parameter invalid", 400)
		return
	}
	et, err := strconv.Atoi(endTime[0])
	if err != nil {
		s.logger.Info("parameter invalid:%#v", query)
		s.jsonError(w, "parameter invalid", 400)
		return
	}
	wo, err := decimal.NewFromString(weightOI[0])
	if err != nil {
		s.logger.Info("parameter invalid:%#v", query)
		s.jsonError(w, "parameter invalid", 400)
		return
	}
	wm, err := decimal.NewFromString(weightMCB[0])
	if err != nil {
		s.logger.Info("parameter invalid:%#v", query)
		s.jsonError(w, "parameter invalid", 400)
		return
	}
	wf, err := decimal.NewFromString(weightFee[0])
	if err != nil {
		s.logger.Info("parameter invalid:%#v", query)
		s.jsonError(w, "parameter invalid", 400)
		return
	}

	schedule := &mining.Schedule{
		Epoch:     int64(e),
		StartTime: int64(st),
		EndTime:   int64(et),
		WeightFee: wf,
		WeightMCB: wm,
		WeightOI:  wo,
	}
	err = s.upsertSchedule(schedule)
	if err != nil {
		s.logger.Error("failed to write into db, %+v", schedule)
		s.jsonError(w, "internal error", 400)
		return
	}

	resp := make(map[string]string)
	resp["message"] = "Success"
	json.NewEncoder(w).Encode(resp)
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
		err := s.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select(
			"score, fee, oi, stake, timestamp").Where("trader = ? and epoch = ?", trader[0], i).Scan(&rsp).Error
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

func (s *TMServer) upsertSchedule(schedule *mining.Schedule) error {
	err := database.Transaction(s.db, func(tx *gorm.DB) error {
		e := tx.Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "epoch"}},
				UpdateAll: true,
			}).Create(schedule).Error
		if e != nil {
			return e
		} else {
			return nil
		}
	})
	return err
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
