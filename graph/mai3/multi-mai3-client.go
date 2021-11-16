package mai3

import (
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
)

var (
	ErrBlockLengthNotEqualMAI3 = errors.New("length of block graph not equal to mai3 graph")
)

type MultiClient struct {
	clients []GraphInterface
}

type MultiGraphInterface interface {
	GetMultiUsersBasedOnMultiBlockNumbers(blockNumbers []int64) ([][]User, error)
	GetMultiMarkPrices(blockNumbers []int64) (map[string]decimal.Decimal, error)
	GetMai3GraphInterface(index int) (GraphInterface, error)
}

func NewMultiClient(logger logging.Logger, clients []GraphInterface) *MultiClient {
	multiClient := &MultiClient{
		clients: clients,
	}
	logger.Warn("make sure the order of MAI3 graphs is match block graphs")
	return multiClient
}

// GetMultiUsersBasedOnMultiBlockNumbers the order of clients need to match blockNumbers,
// return 2-D users
func (c *MultiClient) GetMultiUsersBasedOnMultiBlockNumbers(blockNumbers []int64) ([][]User, error) {
	if len(blockNumbers) != len(c.clients) {
		return nil, ErrBlockLengthNotEqualMAI3
	}
	g := new(errgroup.Group)

	ret := make([][]User, len(blockNumbers))
	for i, blockGraph := range c.clients {
		i, blockGraph := i, blockGraph
		g.Go(func() error {
			users, err := blockGraph.GetUsersBasedOnBlockNumber(blockNumbers[i])
			if err != nil {
				return err
			}
			if len(users) == 0 {
				return fmt.Errorf("chain(%d) bn(%d) users is empty: graph may error", i, blockNumbers[i])
			}
			ret[i] = users
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

// GetMultiMarkPrices the order of clients need to match blockNumbers
func (c *MultiClient) GetMultiMarkPrices(blockNumbers []int64) (map[string]decimal.Decimal, error) {
	if len(blockNumbers) != len(c.clients) {
		return nil, ErrBlockLengthNotEqualMAI3
	}
	g := new(errgroup.Group)

	ret := make(map[string]decimal.Decimal)
	for i, blockGraph := range c.clients {
		i, blockGraph := i, blockGraph
		g.Go(func() error {
			prices, err := blockGraph.GetMarkPrices(blockNumbers[i])
			if err != nil {
				return err
			}
			for k, v := range prices {
				ret[k] = v
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

func (c *MultiClient) GetMai3GraphInterface(index int) (GraphInterface, error) {
	if index < 0 || len(c.clients) <= index {
		return nil, fmt.Errorf("fail to getMai3GraphInterface index %d", index)
	}
	return c.clients[index], nil
}
