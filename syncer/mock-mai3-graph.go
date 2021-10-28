package syncer

import (
	"errors"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/mai3"
	"github.com/shopspring/decimal"
	"time"
)

var TEST_ERROR = errors.New("test error")

var retMap0 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(0),
	"0xPool-1": decimal.NewFromInt(0),
}
var retMap1 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(100),
	"0xPool-1": decimal.NewFromInt(1000),
}
var retMap2 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(110),
	"0xPool-1": decimal.NewFromInt(1100),
}
var retMap3 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(90),
	"0xPool-1": decimal.NewFromInt(900),
}
var retMap4 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(100),
	"0xPool-1": decimal.NewFromInt(1000),
}
var retMap5 = map[string]decimal.Decimal{
	"0xPool-0": decimal.NewFromInt(100),
	"0xPool-1": decimal.NewFromInt(1000),
}

type MockMAI3Graph1 struct {
	delay time.Duration
}

func (mockMAI3 *MockMAI3Graph1) InBTCInverseContractWhiteList(perpID string) (bool, string) {
	return false, ""
}

func (mockMAI3 *MockMAI3Graph1) InETHInverseContractWhiteList(perpID string) (bool, string) {
	return false, ""
}

func (mockMAI3 *MockMAI3Graph1) InSATSInverseContractWhiteList(perpID string) (bool, string) {
	return false, ""
}

func (mockMAI3 *MockMAI3Graph1) GetPerpIDWithUSDBased(symbol string) (string, error) {
	return "", nil
}

func (mockMAI3 *MockMAI3Graph1) GetUsersBasedOnBlockNumber(blockNumber int64) ([]mai3.User, error) {
	if mockMAI3.delay > 0 {
		time.Sleep(mockMAI3.delay)
	}
	if blockNumber <= 0 {
		return []mai3.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(0),
				UnlockMCBTime:  0,
				MarginAccounts: []*mai3.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 1 {
		return []mai3.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(3),
				UnlockMCBTime:  60 * 60 * 24 * 100, // 100 days
				MarginAccounts: []*mai3.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 2 {
		return []mai3.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(3),
				UnlockMCBTime: 60 * 60 * 24 * 99, // 99 days
				MarginAccounts: []*mai3.MarginAccount{
					{
						ID:       "0xPool-0-0xUser1",
						Position: decimal.NewFromFloat(2),
					},
					{
						ID:       "0xPool-1-0xUser1",
						Position: decimal.NewFromFloat(4),
					},
				},
			},
		}, nil
	}
	if blockNumber == 3 {
		return []mai3.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(3),
				UnlockMCBTime: 60 * 60 * 24 * 98, // 98 days
				MarginAccounts: []*mai3.MarginAccount{
					{
						ID:          "0xPool-0-0xUser1",
						TotalFee:    decimal.NewFromFloat(3),
						VaultFee:    decimal.NewFromFloat(1),
						OperatorFee: decimal.NewFromFloat(1),
						Position:    decimal.NewFromFloat(2),
					},
					{
						ID:          "0xPool-1-0xUser1",
						TotalFee:    decimal.NewFromFloat(7),
						VaultFee:    decimal.NewFromFloat(2),
						OperatorFee: decimal.NewFromFloat(2),
						Position:    decimal.NewFromFloat(4),
					},
				},
			},
		}, nil
	}
	if blockNumber == 4 {
		return []mai3.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(10),
				UnlockMCBTime: 60 * 60 * 24 * 100, // 100 days
				MarginAccounts: []*mai3.MarginAccount{
					{
						ID:          "0xPool-0-0xUser1",
						TotalFee:    decimal.NewFromFloat(5),
						VaultFee:    decimal.NewFromFloat(2),
						OperatorFee: decimal.NewFromFloat(2),
						Position:    decimal.NewFromFloat(7),
					},
					{
						ID:          "0xPool-1-0xUser1",
						TotalFee:    decimal.NewFromFloat(10),
						VaultFee:    decimal.NewFromFloat(3),
						OperatorFee: decimal.NewFromFloat(3),
						Position:    decimal.NewFromFloat(9),
					},
				},
			},
		}, nil
	}
	if blockNumber == 5 {
		return []mai3.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(10),
				UnlockMCBTime: 60 * 60 * 24 * 99, // 99 days
				MarginAccounts: []*mai3.MarginAccount{
					{
						ID:          "0xPool-0-0xUser1",
						TotalFee:    decimal.NewFromFloat(6),
						VaultFee:    decimal.NewFromFloat(2.5),
						OperatorFee: decimal.NewFromFloat(2.5),
						Position:    decimal.NewFromFloat(12),
					},
					{
						ID:          "0xPool-1-0xUser1",
						TotalFee:    decimal.NewFromFloat(12),
						VaultFee:    decimal.NewFromFloat(3),
						OperatorFee: decimal.NewFromFloat(3),
						Position:    decimal.NewFromFloat(24),
					},
				},
			},
		}, nil
	}
	return []mai3.User{}, TEST_ERROR
}

func (mockMAI3 *MockMAI3Graph1) GetMarkPrices(blockNumber int64) (map[string]decimal.Decimal, error) {
	if blockNumber == 0 {
		return retMap0, nil
	}
	if blockNumber == 1 {
		return retMap1, nil
	}
	if blockNumber == 2 {
		return retMap2, nil
	}
	if blockNumber == 3 {
		return retMap3, nil
	}
	if blockNumber == 4 {
		return retMap4, nil
	}
	if blockNumber == 5 {
		return retMap5, nil
	}
	return map[string]decimal.Decimal{}, TEST_ERROR
}

func (mockMAI3 *MockMAI3Graph1) GetMarkPriceWithBlockNumberAddrIndex(blockNumber int64, poolAddr string, perpIndex int) (decimal.Decimal, error) {
	if blockNumber == 0 {
		return decimal.Zero, nil
	}
	if blockNumber == 1 {
		if perpIndex == 0 {
			return retMap1["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap1["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 2 {
		if perpIndex == 0 {
			return retMap2["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap2["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 3 {
		if perpIndex == 0 {
			return retMap3["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap3["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 4 {
		if perpIndex == 0 {
			return retMap4["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap4["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 5 {
		if perpIndex == 0 {
			return retMap5["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap5["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	return decimal.Zero, TEST_ERROR
}

func NewMockMAI3Graph1() *MockMAI3Graph1 {
	return &MockMAI3Graph1{}
}

type MockMAI3Graph2 struct{}

func (mockMAI3 *MockMAI3Graph2) InBTCInverseContractWhiteList(perpID string) (bool, string) {
	return false, ""
}

func (mockMAI3 *MockMAI3Graph2) InETHInverseContractWhiteList(perpID string) (bool, string) {
	return false, ""
}

func (mockMAI3 *MockMAI3Graph2) InSATSInverseContractWhiteList(perpID string) (bool, string) {
	return false, ""
}

func (mockMAI3 *MockMAI3Graph2) GetPerpIDWithUSDBased(symbol string) (string, error) {
	return "", nil
}

func (mockMAI3 *MockMAI3Graph2) GetUsersBasedOnBlockNumber(blockNumber int64) ([]mai3.User, error) {
	if blockNumber <= 0 {
		return []mai3.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(0),
				UnlockMCBTime:  0,
				MarginAccounts: []*mai3.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 1 {
		return []mai3.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(0),
				UnlockMCBTime:  0,
				MarginAccounts: []*mai3.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 2 {
		return []mai3.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(0),
				UnlockMCBTime:  0,
				MarginAccounts: []*mai3.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 3 {
		return []mai3.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(0),
				UnlockMCBTime:  0,
				MarginAccounts: []*mai3.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 4 {
		return []mai3.User{
			{
				ID:             "0xUser1",
				StakedMCB:      decimal.NewFromInt(0),
				UnlockMCBTime:  0,
				MarginAccounts: []*mai3.MarginAccount{},
			},
		}, nil
	}
	if blockNumber == 5 {
		return []mai3.User{
			{
				ID:            "0xUser1",
				StakedMCB:     decimal.NewFromInt(6),
				UnlockMCBTime: 60 * 60 * 24 * 100, // 100 days
				MarginAccounts: []*mai3.MarginAccount{
					{
						ID:          "0xPool-0-0xUser1",
						TotalFee:    decimal.NewFromFloat(40),
						VaultFee:    decimal.NewFromFloat(10),
						OperatorFee: decimal.NewFromFloat(10),
						Position:    decimal.NewFromFloat(100),
					},
				},
			},
		}, nil
	}
	return []mai3.User{}, TEST_ERROR
}

func (mockMAI3 *MockMAI3Graph2) GetMarkPrices(blockNumber int64) (map[string]decimal.Decimal, error) {
	if blockNumber == 0 {
		return retMap0, nil
	}
	if blockNumber == 1 {
		return retMap1, nil
	}
	if blockNumber == 2 {
		return retMap2, nil
	}
	if blockNumber == 3 {
		return retMap3, nil
	}
	if blockNumber == 4 {
		return retMap4, nil
	}
	if blockNumber == 5 {
		return retMap5, nil
	}
	return map[string]decimal.Decimal{}, TEST_ERROR
}

func (mockMAI3 *MockMAI3Graph2) GetMarkPriceWithBlockNumberAddrIndex(blockNumber int64, poolAddr string, perpIndex int) (decimal.Decimal, error) {
	if blockNumber == 0 {
		return decimal.Zero, nil
	}
	if blockNumber == 1 {
		if perpIndex == 0 {
			return retMap1["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap1["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 2 {
		if perpIndex == 0 {
			return retMap2["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap2["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 3 {
		if perpIndex == 0 {
			return retMap3["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap3["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 4 {
		if perpIndex == 0 {
			return retMap4["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap4["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	if blockNumber == 5 {
		if perpIndex == 0 {
			return retMap5["0xPool-0"], nil
		} else if perpIndex == 1 {
			return retMap5["0xPool-1"], nil
		}
		return decimal.Zero, TEST_ERROR
	}
	return decimal.Zero, TEST_ERROR
}

func NewMockMAI3Graph2() *MockMAI3Graph2 {
	return &MockMAI3Graph2{}
}

type MockMultiMAI3Graphs struct {
	clients []mai3.GraphInterface
}

func (mockMultiMAI3Graphs *MockMultiMAI3Graphs) GetMultiUsersBasedOnMultiBlockNumbers(
	blockNumbers []int64) ([][]mai3.User, error) {
	if len(blockNumbers) != len(mockMultiMAI3Graphs.clients) {
		return nil, TEST_ERROR
	}

	ret := make([][]mai3.User, len(blockNumbers))
	for i, bn := range blockNumbers {
		users, err := mockMultiMAI3Graphs.clients[i].GetUsersBasedOnBlockNumber(bn)
		if err != nil {
			return nil, err
		}
		ret[i] = users
	}
	return ret, nil
}

func (mockMultiMAI3Graphs *MockMultiMAI3Graphs) GetMultiMarkPrices(
	blockNumbers []int64) (map[string]decimal.Decimal, error) {
	if len(blockNumbers) != len(mockMultiMAI3Graphs.clients) {
		return nil, TEST_ERROR
	}

	ret := make(map[string]decimal.Decimal)
	for i, bn := range blockNumbers {
		prices, err := mockMultiMAI3Graphs.clients[i].GetMarkPrices(bn)
		if err != nil {
			return nil, err
		}
		for k, v := range prices {
			ret[k] = v
		}
	}
	return ret, nil
}

func (mockMultiMAI3Graphs *MockMultiMAI3Graphs) GetMai3GraphInterface(
	index int) (mai3.GraphInterface, error) {
	if index < 0 || len(mockMultiMAI3Graphs.clients) <= index {
		return nil, fmt.Errorf("fail to getMai3GraphInterface index %d", index)
	}
	return mockMultiMAI3Graphs.clients[index], nil
}

func NewMockMultiMAI3GraphsOneChain() *MockMultiMAI3Graphs {
	multiMai3Clients := MockMultiMAI3Graphs{
		clients: make([]mai3.GraphInterface, 0),
	}
	multiMai3Clients.clients = append(multiMai3Clients.clients, NewMockMAI3Graph1())
	return &multiMai3Clients
}

func NewMockMultiMAI3GraphsMultiChain() *MockMultiMAI3Graphs {
	multiMai3Clients := MockMultiMAI3Graphs{
		clients: make([]mai3.GraphInterface, 0),
	}
	multiMai3Clients.clients = append(multiMai3Clients.clients, NewMockMAI3Graph1())
	multiMai3Clients.clients = append(multiMai3Clients.clients, NewMockMAI3Graph2())
	return &multiMai3Clients
}
