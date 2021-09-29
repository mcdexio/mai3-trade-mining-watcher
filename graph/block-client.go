package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
	"strconv"
)

type BlockClient struct {
	logger logging.Logger
	client *utils.Client
}

func NewBlockClient(logger logging.Logger, url string) *BlockClient {
	logger.Info("New block client with url %s", url)
	return &BlockClient{
		logger: logger,
		client: utils.NewHttpClient(utils.DefaultTransport, logger, url),
	}
}

type Block struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Timestamp string `json:"timestamp"`
}

// GetTimestampToBlockNumber which is the closest but less than or equal to timestamp
func (b *BlockClient) GetTimestampToBlockNumber(timestamp int64) (int64, error) {
	b.logger.Debug("get block number which is the closest but less than or equal to timestamp %d", timestamp)
	query := `{
		blocks(
			first:1, orderBy: number, orderDirection: asc, 
			where: {timestamp_gt: %d}
		) {
			id
			number
			timestamp
		}
	}`
	var response struct {
		Data struct {
			Blocks []*Block
		}
	}
	// return err when can't get block number in three times
	if err := b.queryGraph(&response, query, timestamp); err != nil {
		return -1, fmt.Errorf("fail to transform timestamp to block number in three times %w", err)
	}

	if len(response.Data.Blocks) != 1 {
		return -1, fmt.Errorf("length of block of response is not equal to 1: expect=1, actual=%v, timestamp=%v", len(response.Data.Blocks), timestamp)
	}
	number, err := strconv.Atoi(response.Data.Blocks[0].Number)
	if err != nil {
		return -1, fmt.Errorf("failed to convert block number from string to int err:%s", err)
	}
	return int64(number - 1), nil
}

// queryGraph return err if failed to get response from graph in three times
func (b *BlockClient) queryGraph(resp interface{}, query string, args ...interface{}) error {
	var params struct {
		Query string `json:"query"`
	}
	params.Query = fmt.Sprintf(query, args...)
	for i := 0; i < 3; i++ {
		err, code, res := b.client.Post(nil, params, nil)
		if err != nil {
			b.logger.Error("fail to post http request %w", err)
			continue
		} else if code/100 != 2 {
			b.logger.Error("unexpected http response: %v", code)
			continue
		}
		err = json.Unmarshal(res, &resp)
		if err != nil {
			b.logger.Error("failed to unmarshal %w", err)
			continue
		}
		// success
		return nil
	}
	return errors.New("failed to query block graph in three times")
}
