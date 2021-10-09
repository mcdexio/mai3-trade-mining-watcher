package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type epochStats struct {
	epoch           int64
	totalTrader     int64
	totalFee        decimal.Decimal
	totalStakeScore decimal.Decimal
	totalOI         decimal.Decimal
	totalScore      decimal.Decimal
}

type EpochTotalStatsResp struct {
	TotalTrader     int64  `json:"totalTrader"`
	TotalFee        string `json:"totalFee"`
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

type EpochScoreResp struct {
	Fee          string `json:"fee"`
	AverageOI    string `json:"averageOI"`
	AverageStake string `json:"averageStake"`
	Score        string `json:"score"`
	Proportion   string `json:"proportion"`
	TotalScore   string `json:"totalScore"`
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
		totalStakeScore: decimal.Zero,
		totalOI:         decimal.Zero,
		totalScore:      decimal.Zero,
	}
	mux := http.NewServeMux()
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
	// start from default epoch start time
	var ss *mining.Schedule
	if err := db.Where("end_time<?", time.Now().Unix()).Order("epoch desc").First(&ss).Error; err != nil {
		return nil, fmt.Errorf("fail to found epoch config %w", err)
	}
	return ss, nil
}

func (s *TMServer) calculateStat(info *mining.UserInfo, schedule *mining.Schedule) (fee decimal.Decimal, oi decimal.Decimal, stake decimal.Decimal) {
	if info == nil || schedule == nil {
		return
	}
	minuteCeil := int64(math.Floor((float64(info.Timestamp) - float64(schedule.StartTime)) / 60.0))
	elapsed := decimal.NewFromInt(minuteCeil) // Minutes
	totalEpochMinutes := decimal.NewFromFloat(math.Ceil(float64(schedule.EndTime-schedule.StartTime) / 60))
	remains := totalEpochMinutes.Sub(elapsed)

	// oi
	oi = info.AccPosValue.Add(info.CurPosValue.Mul(remains)).Div(totalEpochMinutes)
	// stake
	stake = info.AccStakeScore.Add(info.EstimatedStakeScore).Div(totalEpochMinutes)
	// fee
	fee = info.AccFee.Sub(info.InitFee)
	return
}

func (s *TMServer) calculateTotalStats() (err error) {
	s.logger.Info("calculateTotalStats.....")
	lastEpoch := s.nowEpoch

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

	s.logger.Info("calculate total status from %d to this epoch %d", lastEpoch, s.nowEpoch)
	for i := lastEpoch; i <= s.nowEpoch; i++ {
		var traders []*mining.UserInfo
		err = s.db.Model(&mining.UserInfo{}).Where("epoch = ?", i).Scan(&traders).Error
		if err != nil {
			return
		}
		count := len(traders)
		s.logger.Info("Epoch %d has %d traders", i, count)
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
			fee, oi, stake := s.calculateStat(t, sch)
			epochStat.totalOI = epochStat.totalOI.Add(oi)
			epochStat.totalStakeScore = epochStat.totalStakeScore.Add(stake)
			epochStat.totalFee = epochStat.totalFee.Add(fee)
			epochStat.totalScore = epochStat.totalScore.Add(t.Score)
		}
		s.history[int(i)] = epochStat
		s.logger.Info("Epoch %d: totalFee %s, totalStakeScore %s, totalOpenInterest %s, totalScore %s",
			i, epochStat.totalFee.String(), epochStat.totalStakeScore.String(), epochStat.totalOI.String(), epochStat.totalScore.String(),
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
			"trader = ? and epoch = ?", traderID, i).First(&rsp).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Info("user %s not found in db", traderID)
			} else {
				s.logger.Error("failed to get value from user info table err=%s", err)
				s.jsonError(w, "internal error", 400)
				return
			}
		}
		s.logger.Info("Epoch %d: user info %+v", i, rsp)
		s.logger.Info("Epoch %d: totalScore %s", i, s.history[i].totalScore)
		stats, match := s.history[i]
		if !match {
			s.logger.Error("failed to get stats %+v", s.history)
			s.jsonError(w, "internal error", 400)
			return
		}

		sch, err := s.getScheduleWithEpoch(s.db, i)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Error("failed to get epoch %d from schedule table err=%s", i, err)
			}
			s.logger.Error("failed to get epoch %d from schedule table err=%s", i, err)
			s.jsonError(w, "internal error", 400)
			return
		}

		var proportion string
		totalScore := stats.totalScore
		if totalScore.IsZero() {
			proportion = "0"
		} else {
			proportion = (rsp.Score.Div(totalScore)).String()
		}

		fee, oi, stake := s.calculateStat(&rsp, sch)

		resp := EpochScoreResp{
			Fee:          fee.String(),
			AverageOI:    oi.String(),
			AverageStake: stake.String(),
			Score:        rsp.Score.String(),
			Proportion:   proportion,
			TotalScore:   totalScore.String(),
		}
		queryTradingMiningResp[i] = &resp
	}

	for i, resp := range queryTradingMiningResp {
		s.logger.Debug("epoch %d, resp %+v", i, resp)
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
