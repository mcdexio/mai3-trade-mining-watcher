package syncer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
	"github.com/shopspring/decimal"
)

type MarginAccount struct {
	ID       string          `json:"id"`
	Position decimal.Decimal `json:"position"`
}

type User struct {
	ID             string          `json:"id"`
	StakedMCB      decimal.Decimal `json:"stakedMCB"`
	TotalFee       decimal.Decimal `json:"totalFee"`
	UnlockMCBTime  int64           `json:"unlockMCBTime"`
	MarginAccounts []*MarginAccount
}

type Block struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Timestamp string `json:"timestamp"`
}

type MarkPrice struct {
	ID    string           `json:"id"`
	Price *decimal.Decimal `json:"price"`
}

type Request struct {
	Query string `json:"query"`
}

type GraphClient struct {
	MaxRetry      int
	RetryDelay    time.Duration
	Mai3GraphUrl  string
	BlockGraphUrl string

	hc *utils.Client
}

func (c *GraphClient) TimestampToBlockNumber(timestamp int64) (int64, error) {
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
	if err := c.query(c.BlockGraphUrl, &response, query, timestamp); err != nil {
		return -1, fmt.Errorf("fail to transform timestamp to block number in three times %w", err)
	}
	if len(response.Data.Blocks) != 1 {
		return -1, fmt.Errorf("block not found: expect=1, actual=%v, timestamp=%v", len(response.Data.Blocks), timestamp)
	}
	number, err := strconv.Atoi(response.Data.Blocks[0].Number)
	if err != nil {
		return -1, fmt.Errorf("failed to convert block number from string to int err:%s", err)
	}
	return int64(number - 1), nil
}

func (c *GraphClient) GetAllMarkPricesByBlock(bn int64) (map[string]*decimal.Decimal, error) {
	prices := make(map[string]*decimal.Decimal)
	idFilter := "0x0"
	for {
		markPrices, err := c.getMarkPricesByBlock(bn, idFilter)
		if err != nil {
			return prices, nil
		}
		// success get mark prices on block number and idFilter
		for _, p := range markPrices {
			prices[p.ID] = p.Price
		}
		length := len(markPrices)
		if length == 1000 {
			// means there are more markPrices, update idFilter
			idFilter = markPrices[length-1].ID
		} else {
			// means got all markPrices
			return prices, nil
		}
	}
}

func (c *GraphClient) getMarkPricesByBlock(blockNumber int64, id string) ([]MarkPrice, error) {
	query := `{
		markPrices(first: 1000, block: { number: %v }, orderBy: id, orderDirection: asc,
			where: { id_gt: "%s" }
		) {
			id
			price
			timestamp
		}
	}`
	var resp struct {
		Data struct {
			MarkPrices []MarkPrice
		}
	}
	if err := c.query(c.Mai3GraphUrl, &resp, query, blockNumber, id); err != nil {
		return nil, fmt.Errorf("fail to get mark price %w", err)
	}
	return resp.Data.MarkPrices, nil
}

func (c *GraphClient) GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error) {
	var retUser []User

	idFilter := "0x0"
	for {
		users, err := c.getUserWithBlockNumberID(blockNumber, idFilter)
		if err != nil {
			return retUser, err
		}
		// success get user based on block number and idFilter
		retUser = append(retUser, users...)
		length := len(users)
		if length == 1000 {
			// means there are more users, update idFilter
			idFilter = users[length-1].ID
		} else {
			// means got all users
			return retUser, nil
		}
	}
}

// getUserWithBlockNumberID try three times to get users depend on ID with order and filter
func (c *GraphClient) getUserWithBlockNumberID(blockNumber int64, id string) ([]User, error) {
	// s.logger.Debug("Get users based on block number %d and order and filter by ID %s", blockNumber, id)
	query := `{
		users(first: 1000, block: {number: %d}, orderBy: id, orderDirection: asc,
			where: { id_gt: "%s" totalFee_gt: 0}
		) {
			id
			stakedMCB
			unlockMCBTime
			totalFee
			marginAccounts(where: { position_gt: 0}) {
				id
				position
			}
		}
	}`
	var response struct {
		Data struct {
			Users []User
		}
	}
	// try three times for each pagination.
	if err := c.query(c.Mai3GraphUrl, &response, query, blockNumber, id); err != nil {
		return []User{}, errors.New("failed to get users in three times")
	}
	return response.Data.Users, nil
}

func (c *GraphClient) query(
	url string,
	resp interface{},
	query string,
	args ...interface{},
) error {
	req := &Request{Query: fmt.Sprintf(query, args...)}
	for retries := 0; retries < c.MaxRetry; retries++ {
		err, code, res := c.hc.Post(url, nil, req, nil)
		if err == nil && code/100 == 2 {
			if err = json.Unmarshal(res, &resp); err == nil {
				return nil
			}
		}
		if retries < c.MaxRetry-1 {
			time.Sleep(c.RetryDelay)
		}
	}
	return fmt.Errorf("failed to query graph data in %v retries", c.MaxRetry)
}
