package mai3

import (
	"errors"
	"fmt"
	"github.com/shopspring/decimal"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
)

var (
	ErrBlockLengthNotEqualMAI3 = errors.New("length of block graph not equal to mai3 graph")
)

type MultiClient struct {
	clients []*Client
}

func NewMultiClient(logger logging.Logger, urls ...string) *MultiClient {
	multiClient := &MultiClient{
		clients: make([]*Client, 0),
	}
	logger.Warn("make sure the order of MAI3 graphs is match block graphs")
	for _, url := range urls {
		multiClient.clients = append(multiClient.clients, NewClient(logger, url))
	}
	return multiClient
}

// GetMultiUsersBasedOnMultiBlockNumbers the order of clients need to match blockNumbers,
// return 2-D users
func (c *MultiClient) GetMultiUsersBasedOnMultiBlockNumbers(blockNumbers []int64) ([][]User, error) {
	if len(blockNumbers) != len(c.clients) {
		return nil, ErrBlockLengthNotEqualMAI3
	}

	ret := make([][]User, len(blockNumbers))
	for i, bn := range blockNumbers {
		users, err := c.clients[i].GetUsersBasedOnBlockNumber(bn)
		if err != nil {
			return nil, err
		}
		ret[i] = users
	}
	return ret, nil
}

// GetMultiMarkPrices the order of clients need to match blockNumbers
func (c *MultiClient) GetMultiMarkPrices(blockNumbers []int64) (map[string]decimal.Decimal, error) {
	if len(blockNumbers) != len(c.clients) {
		return nil, ErrBlockLengthNotEqualMAI3
	}

	ret := make(map[string]decimal.Decimal)
	for i, bn := range blockNumbers {
		prices, err := c.clients[i].GetMarkPrices(bn)
		if err != nil {
			return nil, err
		}
		for k, v := range prices {
			ret[k] = v
		}
	}
	return ret, nil
}

func (c *MultiClient) GetMai3GraphInterface(index int) (Interface, error) {
	if index < 0 || len(c.clients) <= index {
		return nil, fmt.Errorf("fail to getMai3GraphInterface index %d", index)
	}
	return c.clients[index], nil
}
