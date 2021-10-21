package mai3

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
	"github.com/shopspring/decimal"
)

type Client struct {
	logger logging.Logger
	client *utils.Client
}

type User struct {
	ID             string          `json:"id"`
	StakedMCB      decimal.Decimal `json:"stakedMCB"`
	UnlockMCBTime  int64           `json:"unlockMCBTime"`
	MarginAccounts []*MarginAccount
}

type MarginAccount struct {
	ID          string          `json:"id"`
	Position    decimal.Decimal `json:"position"`
	TotalFee    decimal.Decimal `json:"totalFee"`
	VaultFee    decimal.Decimal `json:"vaultFee"`
	OperatorFee decimal.Decimal `json:"operatorFee"`
}

type MarkPrice struct {
	ID    string          `json:"id"`
	Price decimal.Decimal `json:"price"`
}

type Interface interface {
	GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error)
	GetMarkPrices(blockNumber int64) (map[string]decimal.Decimal, error)
	GetMarkPriceWithBlockNumberAddrIndex(
		blockNumber int64, poolAddr string, perpIndex int) (decimal.Decimal, error)
}

func NewClient(logger logging.Logger, url string) *Client {
	logger.Info("New MAI3 graph client with url %s", url)
	return &Client{
		logger: logger,
		client: utils.NewHttpClient(utils.DefaultTransport, logger, url),
	}
}

// GetMarkPrices get mark prices with block number. return map[markPriceID]price
func (m *Client) GetMarkPrices(blockNumber int64) (map[string]decimal.Decimal, error) {
	prices := make(map[string]decimal.Decimal)
	idFilter := "0x0"
	for {
		markPrices, err := m.getMarkPricesWithBlockNumberID(blockNumber, idFilter)
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

// getMarkPricesWithBlockNumberID try three times to get markPrices depend on ID with order and filter
func (m *Client) getMarkPricesWithBlockNumberID(blockNumber int64, id string) ([]MarkPrice, error) {
	m.logger.Debug("Get mark price based on block number %d and order and filter by ID %s", blockNumber, id)
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
	if err := m.queryGraph(&resp, query, blockNumber, id); err != nil {
		return nil, fmt.Errorf(
			"fail to get mark price with BN=%d, ID=%s, err=%s", blockNumber, id, err)
	}
	return resp.Data.MarkPrices, nil
}

// queryGraph return err if failed to get response from graph in three times
func (m *Client) queryGraph(resp interface{}, query string, args ...interface{}) error {
	var params struct {
		Query string `json:"query"`
	}
	params.Query = fmt.Sprintf(query, args...)
	for i := 0; i < 3; i++ {
		err, code, res := m.client.Post(nil, params, nil)
		if err != nil {
			m.logger.Error("fail to post http params=%+v err=%s", params, err)
			continue
		} else if code/100 != 2 {
			m.logger.Error("unexpected http response=%v", code)
			continue
		}
		err = json.Unmarshal(res, &resp)
		if err != nil {
			m.logger.Error("fail to unmarshal err=%s", err)
			continue
		}
		// success
		return nil
	}
	return errors.New("fail to query MAI3 graph in three times")
}

// GetUsersBasedOnBlockNumber get users based on blockNumber.
func (m *Client) GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error) {
	m.logger.Debug("Get users based on block number %d", blockNumber)
	var retUser []User

	idFilter := "0x0"
	for {
		users, err := m.getUserWithBlockNumberID(blockNumber, idFilter)
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
func (m *Client) getUserWithBlockNumberID(blockNumber int64, id string) ([]User, error) {
	// s.logger.Debug("Get users based on block number %d and order and filter by ID %s", blockNumber, id)
	query := `{
		users(first: 1000, block: {number: %d}, orderBy: id, orderDirection: asc,
			where: { id_gt: "%s" }
		) {
			id
			stakedMCB
			unlockMCBTime
			marginAccounts(where: { totalFee_gt: 0}) {
				id
				position
				totalFee
				vaultFee
				operatorFee
			}
		}
	}`
	var response struct {
		Data struct {
			Users []User
		}
	}
	// try three times for each pagination.
	if err := m.queryGraph(&response, query, blockNumber, id); err != nil {
		return []User{}, errors.New("failed to get users in three times")
	}
	return response.Data.Users, nil
}

// GetMarkPriceWithBlockNumberAddrIndex get mark price based on block number, pool address, perpetual index.
func (m *Client) GetMarkPriceWithBlockNumberAddrIndex(
	blockNumber int64, poolAddr string, perpIndex int) (decimal.Decimal, error) {
	m.logger.Debug("Get mark price based on block number %d, poolAddr %s, perpIndex %d",
		blockNumber, poolAddr, perpIndex)
	query := `{
		markPrices(first: 1, block: { number: %d }, where: {id: "%s"}) {
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
	id := fmt.Sprintf("%s-%d", poolAddr, perpIndex)
	if err := m.queryGraph(&resp, query, blockNumber, id); err != nil {
		return decimal.Zero, fmt.Errorf(
			"fail to get mark price with BN=%d, poolAddr=%s, perpIndex=%d err=%s",
			blockNumber, poolAddr, perpIndex, err)
	}
	if len(resp.Data.MarkPrices) == 0 {
		return decimal.Zero, fmt.Errorf("empty mark price resp=%+v", resp)
	}
	return resp.Data.MarkPrices[0].Price, nil
}
