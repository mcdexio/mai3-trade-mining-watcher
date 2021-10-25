package block

import "github.com/mcdexio/mai3-trade-mining-watcher/common/logging"

type MultiClient struct {
	clients []BlockInterface
}

type MultiBlockInterface interface {
	GetMultiBlockNumberWithTS(timestamp int64) ([]int64, error)
}

func NewMultiClient(logger logging.Logger, clients []BlockInterface) *MultiClient {
	multiClient := &MultiClient{
		clients: clients,
	}
	logger.Warn("make sure the order of block graphs is match MAI3 graphs")
	return multiClient
}

func (c *MultiClient) GetMultiBlockNumberWithTS(timestamp int64) ([]int64, error) {
	var ret []int64
	for _, blockGraph := range c.clients {
		bn, err := blockGraph.GetBlockNumberWithTS(timestamp)
		if err != nil {
			return nil, err
		}
		ret = append(ret, bn)
	}
	return ret, nil
}
