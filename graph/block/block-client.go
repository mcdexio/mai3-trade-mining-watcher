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
	logger  logging.Logger
	client  *utils.Client
	url     string
	tsCache map[int64]int64
}

func NewClient(logger logging.Logger, url string) *Client {
	logger.Info("New block graph client with url %s", url)
	if url == "" {
		return nil
	}
	return &Client{
		logger:  logger,
		client:  utils.NewHttpClient(utils.DefaultTransport, logger, url),
		url:     url,
		tsCache: make(map[int64]int64),
	}
}

type Block struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Timestamp string `json:"timestamp"`
}

// // GetBlockNumberWithTS which is the closest but less than or equal to timestamp
// func (b *Client) GetBlockNumberWithTS(timestamp int64) (int64, error) {
// 	startTime := time.Now().Unix()
// 	defer func() {
// 		endTime := time.Now().Unix()
// 		b.logger.Info("leave GetBlockNumberWithTS which is the closest but <= @ts:%d, takes %d seconds: url %s", timestamp, endTime-startTime, b.url)
// 	}()
//
// 	if bn, match := b.tsCache[timestamp]; match {
// 		b.logger.Debug("match in tsCache")
// 		return bn, nil
// 	}
// 	b.logger.Debug("didn't match get from graph")
//
// 	query := `
// 		b%d: blocks(
// 			first:1, orderBy: number, orderDirection: asc,
// 			where: {timestamp_gte: %d}
// 		) {
// 			number
// 		}
// 	`
// 	bigQuery := `{`
// 	for t := timestamp; t < timestamp + 2*60; t+=60 {
// 		bigQuery += fmt.Sprintf(query, t, t)
// 	}
// 	bigQuery += `}`
// 	var response struct {
// 		Data struct {
// 			b1636149960 map[string][]map[string]string
// 			b1636150020 map[string][]map[string]string
// 		}
// 	}
// 	// return err when can't get block number in three times
// 	if err := b.queryGraph(&response, bigQuery); err != nil {
// 		return -1, err
// 	}
//
// 	b.logger.Info("response.Data %+v", response.Data)
//
// 	return int64(- 1), nil
// }

// GetBlockNumberWithTS which is the closest but less than or equal to timestamp
func (b *Client) GetBlockNumberWithTS(timestamp int64) (int64, error) {
	startTime := time.Now().Unix()
	defer func() {
		endTime := time.Now().Unix()
		b.logger.Info("leave GetBlockNumberWithTS which @ts:%d, takes %d seconds: url %s", timestamp, endTime-startTime, b.url)
	}()

	timestamp = norm(timestamp)
	if bn, match := b.tsCache[timestamp]; match {
		b.logger.Debug("match in tsCache")
		return bn, nil
	}
	b.logger.Debug("didn't match ts %d get from graph", timestamp)

	var response struct {
		Data struct {
			Blocks []*Block
		}
	}

	query := `{
		blocks(
			first:1000, orderBy: number, orderDirection: asc,
			where: {timestamp_gte: %d}
		) {
			number
			timestamp
		}
	}`

	// return err when can't get block number in three times
	if err := b.queryGraph(&response, query, timestamp); err != nil {
		return -1, err
	}

	for _, block := range response.Data.Blocks {
		ts, err := strconv.Atoi(block.Timestamp)
		if err != nil {
			return -1, fmt.Errorf("fail to get ts %s from string err=%s", block.Timestamp, err)
		}
		tsInt64 := norm(int64(ts))
		if _, match := b.tsCache[tsInt64]; match {
			continue
		} else {
			// because number is asc and after norm, so get the closest ts as bn
			var bn int
			bn, err = strconv.Atoi(block.Number)
			if err != nil {
				return -1, fmt.Errorf("fail to get bn %s from string err=%s", block.Number, err)
			}
			b.tsCache[tsInt64] = int64(bn)
		}
	}

	return b.tsCache[timestamp], nil
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
			b.logger.Error("unexpected http params=%+v, response=%v, url %s", params, code, b.url)
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

func norm(ts int64) int64 {
	return ts - ts%60
}
