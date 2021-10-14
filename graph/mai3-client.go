package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	utils "github.com/mcdexio/mai3-trade-mining-watcher/utils/http"
	"github.com/shopspring/decimal"
)

type MAI3Client struct {
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
	ID             string          `json:"id"`
	Position       decimal.Decimal `json:"position"`
	TotalFee       decimal.Decimal `json:"totalFee"`
	LpFee          decimal.Decimal `json:"lpFee"`
	VaultFee       decimal.Decimal `json:"vaultFee"`
	OperatorFee    decimal.Decimal `json:"operatorFee"`
	ReferralRebate decimal.Decimal `json:"referralRebate"`
}

type MarkPrice struct {
	ID    string          `json:"id"`
	Price decimal.Decimal `json:"price"`
}

type MAI3Interface interface {
	GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error)
	GetMarkPrices(blockNumber int64) (map[string]decimal.Decimal, error)
	GetMarkPriceWithBlockNumberAddrIndex(
		blockNumber int64, poolAddr string, perpetualIndex int) (decimal.Decimal, error)
}

func NewMAI3Client(logger logging.Logger, url string) *MAI3Client {
	logger.Info("New MAI3 client with url %s", url)
	return &MAI3Client{
		logger: logger,
		client: utils.NewHttpClient(utils.DefaultTransport, logger, url),
	}
}

// GetMarkPrices get mark prices with block number. return map[markPriceID]price
func (m *MAI3Client) GetMarkPrices(blockNumber int64) (map[string]decimal.Decimal, error) {
	m.logger.Debug("Get mark price based on block number %d", blockNumber)
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
func (m *MAI3Client) getMarkPricesWithBlockNumberID(blockNumber int64, id string) ([]MarkPrice, error) {
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
		return nil, fmt.Errorf("fail to get mark price %w", err)
	}
	return resp.Data.MarkPrices, nil
}

// queryGraph return err if failed to get response from graph in three times
func (m *MAI3Client) queryGraph(resp interface{}, query string, args ...interface{}) error {
	var params struct {
		Query string `json:"query"`
	}
	params.Query = fmt.Sprintf(query, args...)
	for i := 0; i < 3; i++ {
		err, code, res := m.client.Post(nil, params, nil)
		if err != nil {
			m.logger.Error("fail to post http request %w", err)
			continue
		} else if code/100 != 2 {
			m.logger.Error("unexpected http response: %v", code)
			continue
		}
		err = json.Unmarshal(res, &resp)
		if err != nil {
			m.logger.Error("failed to unmarshal %w", err)
			continue
		}
		// success
		return nil
	}
	return errors.New("failed to query block graph in three times")
}

// GetUsersBasedOnBlockNumber get users based on blockNumber.
func (m *MAI3Client) GetUsersBasedOnBlockNumber(blockNumber int64) ([]User, error) {
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
func (m *MAI3Client) getUserWithBlockNumberID(blockNumber int64, id string) ([]User, error) {
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
func (m *MAI3Client) GetMarkPriceWithBlockNumberAddrIndex(
	blockNumber int64, poolAddr string, perpetualIndex int) (decimal.Decimal, error) {
	m.logger.Debug("Get mark price based on block number %d, poolAddr %s, perpetualIndex %d",
		blockNumber, poolAddr, perpetualIndex)
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
	id := fmt.Sprintf("%s-%d", poolAddr, perpetualIndex)
	if err := m.queryGraph(&resp, query, blockNumber, id); err != nil {
		return decimal.Zero, fmt.Errorf("fail to get mark price %w", err)
	}
	if len(resp.Data.MarkPrices) == 0 {
		return decimal.Zero, errors.New("empty mark price")
	}
	return resp.Data.MarkPrices[0].Price, nil
}
