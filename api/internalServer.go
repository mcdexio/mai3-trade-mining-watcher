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

type InternalServer struct {
	ctx      context.Context
	logger   logging.Logger
	db       *gorm.DB
	mux      *http.ServeMux
	server   *http.Server
	nowEpoch int
	score    map[int]decimal.Decimal
}

func NewInternalServer(ctx context.Context, logger logging.Logger) *InternalServer {
	server := &InternalServer{
		logger: logger,
		db:     database.GetDB(),
		ctx:    ctx,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthCheckup", server.OnQueryHealthCheckup)
	mux.HandleFunc("/setEpoch", server.OnQuerySetEpoch)
	server.server = &http.Server{
		Addr:         ":9453",
		WriteTimeout: time.Second * 25,
		Handler:      mux,
	}
	return server
}

func (s *InternalServer) OnQueryHealthCheckup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	resp := make(map[string]string)
	resp["message"] = "alive"
	json.NewEncoder(w).Encode(resp)
}

func (s *InternalServer) Shutdown() error {
	return s.server.Shutdown(s.ctx)
}

func (s *InternalServer) Run() error {
	s.logger.Info("Starting trading mining internal httpserver")
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
		select {
		case <-s.ctx.Done():
			s.logger.Info("Syncer receives shutdown signal.")
			return nil
		}
	}
}

func (s *InternalServer) OnQuerySetEpoch(w http.ResponseWriter, r *http.Request) {
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

func (s *InternalServer) upsertSchedule(schedule *mining.Schedule) error {
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

func (s *InternalServer) jsonError(w http.ResponseWriter, err interface{}, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	var msg struct {
		Error string `json:"error"`
	}
	msg.Error = err.(string)
	json.NewEncoder(w).Encode(msg)
}
