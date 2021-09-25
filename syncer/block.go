package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"time"
)

type BlockSyncer struct {
	httpClient *utils.Client
	ctx        context.Context
	logger     logging.Logger
	graphUrl   string
	startTime  *time.Time
	db         *gorm.DB
}

type Block struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Timestamp string `json:"timestamp"`
}

func NewBlockSyncer(ctx context.Context, logger logging.Logger, blockSyncerGraphUrl string, startTime *time.Time) *BlockSyncer {
	syncer := &BlockSyncer{
		utils.NewHttpClient(transport, logger),
		ctx,
		logger,
		blockSyncerGraphUrl,
		startTime,
		database.GetDB(),
	}
	return syncer
}

func (s *BlockSyncer) Init() {
	now := time.Now()
	s.catchup(s.startTime.Unix(), now.Unix())
}

func (s *BlockSyncer) Run() error {
	ticker := time.NewTicker(60 * time.Second)
	for {
		select {
		// priority by order
		case <-s.ctx.Done():
			ticker.Stop()
			s.logger.Info("BlockSyncer receives shutdown signal.")
			return nil
		case <-ticker.C:
			s.sync()
		}
	}
}

func (s *BlockSyncer) syncFromTo(from, to int64) {
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		blocks(first:500, skip: %d, orderBy: timestamp, orderDirection: asc,
			where: {timestamp_gt: "%d", timestamp_lt: "%d"}
		) {
			id
			number
			timestamp
		}
	}`
	skip := 0
	startTime := from
	endTime := to
	latestTime := int64(0)
	for {
		params.Query = fmt.Sprintf(queryFormat, skip, startTime, endTime)
		err, code, res := s.httpClient.Post(s.graphUrl, nil, params, nil)
		if err != nil || code != 200 {
			s.logger.Info("Failed to get block info err:%s, code:%d", err, code)
		}

		var response struct {
			Data struct {
				Blocks []*Block
			}
		}
		err = json.Unmarshal(res, &response)
		if err != nil {
			s.logger.Error("Failed to unmarshal err:%s", err)
			return
		}

		for _, block := range response.Data.Blocks {
			b := s.marshal(block)
			if b == nil {
				continue
			}
			s.upsertBlockIntoDB(b)
			if b.Timestamp > latestTime {
				latestTime = b.Timestamp
			}
		}
		if len(response.Data.Blocks) == 500 {
			// means there are more data to get
			skip += 500
			if skip == 5000 {
				// TheGraph only support skip == 5000, so update startTime for filter
				startTime = latestTime
				skip = 0
				s.logger.Debug("skip %d, startTime %d", skip, startTime)
			}
		} else {
			// we have got all
			break
		}
	}
}

// catchup from startTime to endTime.
func (s *BlockSyncer) catchup(startTime int64, endTime int64) {
	var blockProgress mining.Progress
	err := s.db.Model(mining.Progress{}).Scan(&blockProgress).Where("table_name = block").Error
	if err != nil || blockProgress.TableName == "" {
		s.logger.Warn("Progress is empty")
		s.logger.Info("sync from startTime %d to %d", startTime, endTime)
		s.syncFromTo(startTime, endTime)
		s.db.Create(&mining.Progress{
			TableName: types.Block,
			From:      startTime,
			To:        endTime,
		})
		return
	}

	if startTime < blockProgress.From {
		s.logger.Info("sync from startTime %d to %d", startTime, blockProgress.From)
		s.syncFromTo(startTime, blockProgress.From)
		s.db.Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "table_name"}},
				DoUpdates: clause.AssignmentColumns([]string{"from"}),
			},
		).Create(&mining.Progress{TableName: types.Block, From: startTime})
	}

	if endTime > blockProgress.To {
		s.logger.Info("sync from startTime %d to %d", blockProgress.To, endTime)
		s.syncFromTo(blockProgress.To, endTime)
		s.db.Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "table_name"}},
				DoUpdates: clause.AssignmentColumns([]string{"to"}),
			},
		).Create(&mining.Progress{TableName: types.Block, To: endTime})
	}
}

// sync latest 30 blocks per 60 seconds.
func (s *BlockSyncer) sync() {
	s.logger.Info("Block sync per 60 seconds")
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		blocks(first:30, orderBy: timestamp, orderDirection: desc) {
			id
			number
			timestamp
		}
	}`
	params.Query = fmt.Sprintf(queryFormat)
	err, code, res := s.httpClient.Post(s.graphUrl, nil, params, nil)
	if err != nil || code != 200 {
		s.logger.Info("Failed to get block info err:%s, code:%d", err, code)
	}
	var response struct {
		Data struct {
			Blocks []*Block
		}
	}
	err = json.Unmarshal(res, &response)
	if err != nil {
		s.logger.Error("Failed to unmarshal err:%s", err)
		return
	}
	for _, block := range response.Data.Blocks {
		b := s.marshal(block)
		if b == nil {
			continue
		}
		s.upsertBlockIntoDB(b)
	}
}

// Insert a block into db, update a block if block hash is already there.
func (s *BlockSyncer) upsertBlockIntoDB(newBlock *mining.Block) {
	s.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(newBlock)
}

func (s *BlockSyncer) marshal(block *Block) *mining.Block {
	number, err := strconv.Atoi(block.Number)
	if err != nil {
		s.logger.Error("Failed to convert block number from string to int err:%s", err)
		return nil
	}
	timestamp, err := strconv.Atoi(block.Timestamp)
	if err != nil {
		s.logger.Error("Failed to convert block timestamp from string to int err:%s", err)
		return nil
	}
	b := &mining.Block{
		ID:        block.ID,
		Number:    int64(number),
		Timestamp: int64(timestamp),
	}
	return b
}
