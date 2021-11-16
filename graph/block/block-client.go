package block

import (
	"encoding/json"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
	"strconv"
	"time"
)

type Errors []struct {
	Message string
}

func (e Errors) Error() string {
	return e[0].Message
}

type BlockInterface interface {
	GetBlockNumberWithTS(timestamp int64) (int64, error)
	GetLatestBlockNumberAndTS() (int64, int64, error)
	GetTimestampWithBN(blockNumber int64) (int64, error)
}

type Client struct {
	logger logging.Logger
	client *utils.Client
}

func NewClient(logger logging.Logger, url string) *Client {
	logger.Info("New block graph client with url %s", url)
	if url == "" {
		return nil
	}
	return &Client{
		logger: logger,
		client: utils.NewHttpClient(utils.DefaultTransport, logger, url),
	}
}

type Block struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Timestamp string `json:"timestamp"`
}

// GetBlockNumberWithTS which is the closest but less than or equal to timestamp
func (b *Client) GetBlockNumberWithTS(timestamp int64) (int64, error) {
	startTime := time.Now().Unix()
	defer func() {
		endTime := time.Now().Unix()
		b.logger.Info("leave GetBlockNumberWithTS which is the closest but <= @ts:%d, takes %d seconds", timestamp, endTime-startTime)
	}()
	query := `{
		blocks(
			first:1, orderBy: number, orderDirection: asc, 
			where: {timestamp_gte: %d}
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
		return -1, err
	}

	if len(response.Data.Blocks) != 1 {
		return -1, fmt.Errorf("length of block response: expect=1, actual=%v, timestamp=%v",
			len(response.Data.Blocks), timestamp)
	}
	bn := response.Data.Blocks[0].Number
	number, err := strconv.Atoi(bn)
	if err != nil {
		return -1, fmt.Errorf("fail to get block number %s from string err=%s", bn, err)
	}
	return int64(number - 1), nil
}

// GetTimestampWithBN get timestamp with block number
func (b *Client) GetTimestampWithBN(blockNumber int64) (int64, error) {
	b.logger.Debug("GetTimestampWithBN @bn:%d", blockNumber)
	query := `{
		blocks(first: 1, where: {number: %d}) {
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
	if err := b.queryGraph(&response, query, blockNumber); err != nil {
		return -1, err
	}
	if len(response.Data.Blocks) != 1 {
		return -1, fmt.Errorf("length of block response: expect=1, actual=%v, blockNumber=%v",
			len(response.Data.Blocks), blockNumber)
	}
	ts := response.Data.Blocks[0].Timestamp
	timestamp, err := strconv.Atoi(ts)
	if err != nil {
		return -1, fmt.Errorf("fail to get ts %s from string err=%s", ts, err)
	}
	return int64(timestamp), nil
}

// queryGraph return err if failed to get response from graph in three times
func (b *Client) queryGraph(resp interface{}, query string, args ...interface{}) error {
	var params struct {
		Query string `json:"query"`
	}

	var out struct {
		Errors Errors
	}

	params.Query = fmt.Sprintf(query, args...)
	for i := 0; i < 3; i++ {
		err, code, res := b.client.Post(nil, params, nil)
		if err != nil {
			b.logger.Error("fail to post http params=%+v err=%s", params, err)
			continue
		} else if code/100 != 2 {
			b.logger.Error("unexpected http params=%+v, response=%v", params, code)
			continue
		}
		err = json.Unmarshal(res, &out)
		if err != nil {
			b.logger.Error("fail to decode error=%+v, err=%s", res, err)
			return err
		}
		if len(out.Errors) > 0 {
			return out.Errors
		}
		err = json.Unmarshal(res, &resp)
		if err != nil {
			b.logger.Error("fail to unmarshal result=%+v, err=%s", res, err)
			continue
		}
		// success
		return nil
	}
	return fmt.Errorf("fail to query block graph in three times")
}

// GetLatestBlockNumberAndTS get the closest block number.
func (b *Client) GetLatestBlockNumberAndTS() (int64, int64, error) {
	query := `{
		blocks(first: 1, orderBy: number, orderDirection: desc) {
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
	if err := b.queryGraph(&response, query); err != nil {
		return -1, -1, err
	}

	if len(response.Data.Blocks) != 1 {
		return -1, -1, fmt.Errorf("length of block response: expect=1, actual=%v",
			len(response.Data.Blocks))
	}
	bn := response.Data.Blocks[0].Number
	number, err := strconv.Atoi(bn)
	if err != nil {
		return -1, -1, fmt.Errorf("fail to get block number %s from string err=%s", bn, err)
	}
	ts := response.Data.Blocks[0].Timestamp
	timestamp, err := strconv.Atoi(ts)
	if err != nil {
		return -1, -1, fmt.Errorf("fail to get ts %s from string err=%s", ts, err)
	}
	return int64(number), int64(timestamp), nil
}
