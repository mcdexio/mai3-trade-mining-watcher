package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"gorm.io/gorm"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/syncer"
	"github.com/shopspring/decimal"
)

type epochStats struct {
	epoch           int64
	totalTrader     int64
	totalFee        decimal.Decimal
	totalDaoFee     decimal.Decimal
	totalStakeScore decimal.Decimal
	totalOI         decimal.Decimal
	totalScore      decimal.Decimal
}

type EpochTotalStatsResp struct {
	TotalTrader     int64  `json:"totalTrader"`
	TotalFee        string `json:"totalFee"`
	TotalDaoFee     string `json:"totalDaoFee"`
	TotalStakeScore string `json:"totalStakeScore"`
	TotalOI         string `json:"totalOI"`
	TotalScore      string `json:"totalScore"`
}

type TMServer struct {
	ctx      context.Context
	logger   logging.Logger
	db       *gorm.DB
	mux      *http.ServeMux
	server   *http.Server
	nowEpoch int64
	history  map[int]epochStats
}

type MultiEpochScoreResp struct {
	TotalFee     map[string]string `json:"totalFee"`
	DaoFee       map[string]string `json:"daoFee"`
	AverageOI    map[string]string `json:"averageOI"`
	AverageStake map[string]string `json:"averageStake"`
	Score        string            `json:"score"`
	Proportion   string            `json:"proportion"`
}

type EpochScoreResp struct {
	TotalFee     string `json:"totalFee"`
	DaoFee       string `json:"daoFee"`
	AverageOI    string `json:"averageOI"`
	AverageStake string `json:"averageStake"`
	Score        string `json:"score"`
	Proportion   string `json:"proportion"`
}

func NewTMServer(ctx context.Context, logger logging.Logger) *TMServer {
	tmServer := &TMServer{
		logger:  logger,
		db:      database.GetDB(),
		ctx:     ctx,
		history: make(map[int]epochStats),
	}
	tmServer.history[0] = epochStats{
		epoch:           0,
		totalTrader:     0,
		totalFee:        decimal.Zero,
		totalDaoFee:     decimal.Zero,
		totalStakeScore: decimal.Zero,
		totalOI:         decimal.Zero,
		totalScore:      decimal.Zero,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/multiScore", tmServer.OnQueryMultiScore)
	mux.HandleFunc("/score", tmServer.OnQueryScore)
	mux.HandleFunc("/totalStats", tmServer.OnQueryTotalStats)
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
		if err := s.calculateTotalStats(); err == nil {
			break
		} else {
			s.logger.Warn("server error occurs while running: %s", err)
			time.Sleep(30 * time.Second)
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
				if err := s.calculateTotalStats(); err == nil {
					break
				} else {
					s.logger.Warn("server error occurs while running: %s", err)
				}
			}
		}
	}
}

func (s *TMServer) getScheduleWithEpoch(db *gorm.DB, epoch int) (sch *mining.Schedule, err error) {
	err = db.Model(mining.Schedule{}).Where("epoch = ?", epoch).First(&sch).Error
	if err != nil {
		return
	}
	return
}

func (s *TMServer) getLatestSchedule(db *gorm.DB) (*mining.Schedule, error) {
	var ss *mining.Schedule
	if err := db.Where("start_time < ?", time.Now().Unix()).Order("epoch desc").First(&ss).Error; err != nil {
		return nil, fmt.Errorf("fail to found epoch config %w", err)
	}
	return ss, nil
}

func (s *TMServer) calculateStat(info *mining.UserInfo, schedule *mining.Schedule) (totalFee, daoFee, oi, stake decimal.Decimal) {
	if info == nil || schedule == nil {
		return
	}

	remains := syncer.GetRemainMinutes(info.Timestamp, schedule)

	totalEpochMinutes := math.Ceil(float64(schedule.EndTime-schedule.StartTime) / 60)
	totalEpochMinutesDecimal := decimal.NewFromFloat(totalEpochMinutes)

	// oi
	future := info.CurPosValue.Mul(remains)
	oi = (info.AccPosValue.Add(future)).Div(totalEpochMinutesDecimal)
	// stake
	stake = (info.AccStakeScore.Add(info.CurStakeScore).Add(info.EstimatedStakeScore)).Div(
		totalEpochMinutesDecimal)
	// fee
	daoFee = info.AccFee.Sub(info.InitFee)
	totalFee = info.AccTotalFee.Sub(info.InitTotalFee)
	return
}

func (s *TMServer) calculateTotalStats() (err error) {
	var schedule *mining.Schedule
	for {
		if schedule, err = s.getLatestSchedule(s.db); err == nil {
			// success
			break
		} else {
			s.logger.Warn("server error occurs while running: %s", err)
			time.Sleep(10 * time.Second)
		}
	}
	s.nowEpoch = schedule.Epoch

	s.logger.Info("calculate total status from 0 to this epoch %d", s.nowEpoch)
	for i := int64(0); i <= s.nowEpoch; i++ {
		var traders []*mining.UserInfo
		err = s.db.Model(&mining.UserInfo{}).Where("epoch = ? and chain = 'total'", i).Scan(&traders).Error
		if err != nil {
			return
		}
		count := len(traders)
		epochStat := epochStats{}
		if count == 0 {
			s.history[int(i)] = epochStat
			continue
		}

		var sch *mining.Schedule
		sch, err = s.getScheduleWithEpoch(s.db, int(i))
		if err != nil {
			s.logger.Error("failed to get epoch %d from schedule table err=%s", i, err)
			return err
		}

		epochStat.epoch = i
		epochStat.totalTrader = int64(count)
		for _, t := range traders {
			totalFee, daoFee, oi, stake := s.calculateStat(t, sch)
			epochStat.totalOI = epochStat.totalOI.Add(oi)
			epochStat.totalStakeScore = epochStat.totalStakeScore.Add(stake)
			epochStat.totalFee = epochStat.totalFee.Add(totalFee)
			epochStat.totalDaoFee = epochStat.totalDaoFee.Add(daoFee)
			epochStat.totalScore = epochStat.totalScore.Add(t.Score)
		}
		s.history[int(i)] = epochStat
		s.logger.Info("Epoch %d: traders %d, totalFee %s, totalDaoFee %s, "+
			"totalStakeScore %s, totalOpenInterest %s, totalScore %s", i, count,
			epochStat.totalFee.String(), epochStat.totalDaoFee.String(),
			epochStat.totalStakeScore.String(), epochStat.totalOI.String(),
			epochStat.totalScore.String(),
		)
	}
	err = nil
	return
}

func (s *TMServer) OnQueryTotalStats(w http.ResponseWriter, r *http.Request) {
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

	queryTotalStats := make(map[int]*EpochTotalStatsResp)
	for i, history := range s.history {
		queryTotalStats[i] = &EpochTotalStatsResp{
			TotalTrader:     history.totalTrader,
			TotalFee:        history.totalFee.String(),
			TotalStakeScore: history.totalStakeScore.String(),
			TotalOI:         history.totalOI.String(),
			TotalDaoFee:     history.totalDaoFee.String(),
			TotalScore:      history.totalScore.String(),
		}
	}
	json.NewEncoder(w).Encode(queryTotalStats)
}

func (s *TMServer) OnQueryScore(w http.ResponseWriter, r *http.Request) {
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
	if len(trader) != 1 || trader[0] == "" {
		s.logger.Info("invalid or empty parameter:%#v", query)
		s.jsonError(w, "invalid or empty parameter", 400)
		return
	}
	traderID := strings.ToLower(trader[0])
	s.logger.Info("OnQueryScore user id %s", traderID)
	queryTradingMiningResp := make(map[int]*EpochScoreResp)
	for i := 0; i <= int(s.nowEpoch); i++ {
		var rsp mining.UserInfo
		err := s.db.Model(mining.UserInfo{}).Where(
			"trader = ? and epoch = ? and chain = 'total'", traderID, i).First(&rsp).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Info("user %s not found in db epoch %d", traderID, i)
			} else {
				s.logger.Error("failed to get value from user info table err=%s", err)
				s.jsonError(w, "internal error", 400)
				return
			}
		}
		stats, match := s.history[i]
		if !match {
			s.logger.Error("failed to get stats %+v", s.history)
			s.jsonError(w, "internal error", 400)
			return
		}

		s.logger.Debug("Epoch %d: user info %+v, totalScore %s", i, rsp, s.history[i].totalScore.String())

		sch, err := s.getScheduleWithEpoch(s.db, i)
		if err != nil {
			s.logger.Error("failed to get epoch %d from schedule table err=%s", i, err)
			s.jsonError(w, "internal error", 400)
			return
		}

		var proportion string
		totalScore := stats.totalScore
		if totalScore.IsZero() {
			proportion = "0"
		} else {
			if rsp.Score.GreaterThanOrEqual(totalScore) {
				proportion = "1"
			} else {
				proportion = (rsp.Score.Div(totalScore)).String()
			}
		}

		totalFee, daoFee, oi, stake := s.calculateStat(&rsp, sch)
		resp := EpochScoreResp{
			TotalFee:     totalFee.String(),
			DaoFee:       daoFee.String(),
			AverageOI:    oi.String(),
			AverageStake: stake.String(),
			Score:        rsp.Score.String(),
			Proportion:   proportion,
		}
		queryTradingMiningResp[i] = &resp
	}

	for i, resp := range queryTradingMiningResp {
		s.logger.Debug("epoch %d, resp %+v", i, resp)
	}
	json.NewEncoder(w).Encode(queryTradingMiningResp)
}

func (s *TMServer) OnQueryMultiScore(w http.ResponseWriter, r *http.Request) {
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
	if len(trader) != 1 || trader[0] == "" {
		s.logger.Info("invalid or empty parameter:%#v", query)
		s.jsonError(w, "invalid or empty parameter", 400)
		return
	}
	traderID := strings.ToLower(trader[0])
	s.logger.Info("OnQueryScore user id %s", traderID)
	queryTradingMiningResp := make(map[int]*MultiEpochScoreResp)
	for i := 0; i <= int(s.nowEpoch); i++ {
		var rsps []*mining.UserInfo
		err := s.db.Model(mining.UserInfo{}).Where(
			"trader = ? and epoch = ?", traderID, i).Scan(&rsps).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Info("user %s not found in db epoch %d", traderID, i)
			} else {
				s.logger.Error("failed to get value from user info table err=%s", err)
				s.jsonError(w, "internal error", 400)
				return
			}
		}

		stats, match := s.history[i]
		if !match {
			s.logger.Error("failed to get stats %+v", s.history)
			s.jsonError(w, "internal error", 400)
			return
		}

		sch, err := s.getScheduleWithEpoch(s.db, i)
		if err != nil {
			s.logger.Error("failed to get epoch %d from schedule table err=%s", i, err)
			s.jsonError(w, "internal error", 400)
			return
		}

		var resp = MultiEpochScoreResp{}
		if i < int(env.MultiChainEpochStart()) {
			resp = MultiEpochScoreResp{
				TotalFee: map[string]string{
					"total": "0",
				},
				DaoFee: map[string]string{
					"total": "0",
				},
				AverageStake: map[string]string{
					"total": "0",
				},
				AverageOI: map[string]string{
					"total": "0",
				},
			}
		} else {
			resp = MultiEpochScoreResp{
				TotalFee: map[string]string{
					"total": "0",
					"0":     "0",
					"1":     "0",
				},
				DaoFee: map[string]string{
					"total": "0",
					"0":     "0",
					"1":     "0",
				},
				AverageStake: map[string]string{
					"total": "0",
					"0":     "0",
					"1":     "0",
				},
				AverageOI: map[string]string{
					"total": "0",
					"0":     "0",
					"1":     "0",
				},
			}
		}

		if i < int(env.MultiChainEpochStart()) {
			// epoch 0, 1
			for _, rsp := range rsps {
				if rsp.Chain == "total" {
					s.marshalEpochAllScoreResp(stats.totalScore, sch, rsp, &resp)
				}
			}
		} else {
			for _, rsp := range rsps {
				s.logger.Debug("userInfo %+v", rsp)
				if rsp.Chain == "total" {
					s.marshalEpochAllScoreResp(stats.totalScore, sch, rsp, &resp)
				} else {
					totalFee, daoFee, oi, stake := s.calculateStat(rsp, sch)
					resp.TotalFee[rsp.Chain] = totalFee.String()
					resp.DaoFee[rsp.Chain] = daoFee.String()
					resp.AverageOI[rsp.Chain] = oi.String()
					resp.AverageStake[rsp.Chain] = stake.String()
				}
			}
		}
		queryTradingMiningResp[i] = &resp
	}
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

func (s *TMServer) marshalEpochAllScoreResp(
	totalScore decimal.Decimal, sch *mining.Schedule, rsp *mining.UserInfo,
	resp *MultiEpochScoreResp) {

	resp.Score = rsp.Score.String()
	var proportion string
	if totalScore.IsZero() {
		proportion = "0"
	} else {
		if rsp.Score.GreaterThanOrEqual(totalScore) {
			proportion = "1"
		} else {
			proportion = (rsp.Score.Div(totalScore)).String()
		}
	}
	resp.Proportion = proportion

	totalFee, daoFee, oi, stake := s.calculateStat(rsp, sch)
	resp.TotalFee["total"] = totalFee.String()
	resp.DaoFee["total"] = daoFee.String()
	resp.AverageOI["total"] = oi.String()
	resp.AverageStake["total"] = stake.String()
}
