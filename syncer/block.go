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
	checkpoint int64
}

func NewBlockSyncer(ctx context.Context, logger logging.Logger, blockSyncerGraphUrl string, startTime *time.Time) *BlockSyncer {
	syncer := &BlockSyncer{
		utils.NewHttpClient(transport, logger),
		ctx,
		logger,
		blockSyncerGraphUrl,
		startTime,
		database.GetDB(),
		0,
	}
	return syncer
}

func (s *BlockSyncer) Run() error {
	s.catchup()
	for {
		select {
		// priority by order
		case <-s.ctx.Done():
			s.logger.Info("BlockSyncer receives shutdown signal.")
			return nil
		case <-time.After(60 * time.Minute):
			s.updateBlockFromCheckpoint()
		case <-time.After(15 * time.Second):
			s.syncPer15Second()
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
				Blocks []struct {
					ID        string `json:"id"`
					Number    string `json:"number"`
					Timestamp string `json:"timestamp"`
				}
			}
		}
		err = json.Unmarshal(res, &response)
		if err != nil {
			s.logger.Error("Failed to unmarshal err:%s", err)
			return
		}

		for _, block := range response.Data.Blocks {
			number, err := strconv.Atoi(block.Number)
			if err != nil {
				s.logger.Error("Failed to convert block number from string to int err:%s", err)
				return
			}
			timestamp, err := strconv.Atoi(block.Timestamp)
			if err != nil {
				s.logger.Error("Failed to convert block timestamp from string to int err:%s", err)
				return
			}
			b := &mining.Block{
				ID:        block.ID,
				Number:    int64(number),
				Timestamp: int64(timestamp),
			}
			s.upsertBlockIntoDB(b)
			if int64(timestamp) > latestTime {
				latestTime = int64(timestamp)
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

// catchup from startTime to now.
func (s *BlockSyncer) catchup() {
	s.logger.Info("Catchup block from %s until now", s.startTime.String())
	endTime := time.Now().Unix()
	s.checkpoint = endTime-60*60 // 1 hour
	s.syncFromTo(s.startTime.Unix(), endTime)
	s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(
		&mining.Progress{
			TableName: types.Block,
			From: s.startTime.Unix(),
			To: endTime,
			Checkpoint: s.checkpoint,
		})
	s.logger.Info("Catchup block done, from %d, checkpoint %d, end %d", s.startTime.Unix(), s.checkpoint, endTime)
}

// syncPerSecond syncs latest 3 blocks per 15 seconds.
func (s *BlockSyncer) syncPer15Second() {
	s.logger.Info("Sync per 15 seconds")
	var params struct {
		Query string `json:"query"`
	}
	queryFormat := `{
		blocks(first:3, orderBy: timestamp, orderDirection: desc) {
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
			Blocks []struct {
				ID        string `json:"id"`
				Number    string `json:"number"`
				Timestamp string `json:"timestamp"`
			}
		}
	}
	err = json.Unmarshal(res, &response)
	if err != nil {
		s.logger.Error("Failed to unmarshal err:%s", err)
		return
	}
	for _, block := range response.Data.Blocks {
		number, err := strconv.Atoi(block.Number)
		if err != nil {
			s.logger.Error("Failed to convert block number from string to int err:%s", err)
			return
		}
		timestamp, err := strconv.Atoi(block.Timestamp)
		if err != nil {
			s.logger.Error("Failed to convert block timestamp from string to int err:%s", err)
			return
		}
		b := &mining.Block{
			ID:        block.ID,
			Number:    int64(number),
			Timestamp: int64(timestamp),
		}
		s.upsertBlockIntoDB(b)
	}
}

// updateBlockFromCheckpoint updates from last checkpoint until one hour ago,
// in order to prevent block rollback.
func (s *BlockSyncer) updateBlockFromCheckpoint() {
	s.logger.Info("Update block from checkpoint %d to one hour ago", s.checkpoint)
	end := time.Now().Unix() - 60*60
	s.syncFromTo(s.checkpoint,end)
	s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "table_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"checkpoint"}),
	}).Create(
		&mining.Progress{
			TableName: types.Block,
			Checkpoint: end,
		})
	s.checkpoint = end
	s.logger.Info("Update checkpoint %d",end)
}

// Insert a block into db, update a block if block hash is already there.
func (s *BlockSyncer) upsertBlockIntoDB(newBlock *mining.Block) {
	s.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(newBlock)
}
