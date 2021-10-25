package block

import "github.com/mcdexio/mai3-trade-mining-watcher/common/logging"

type MultiClient struct {
	clients []*Client
}

func NewMultiClient(logger logging.Logger, urls ...string) *MultiClient {
	multiClient := &MultiClient{
		clients: make([]*Client, 0),
	}
	logger.Warn("make sure the order of block graphs is match MAI3 graphs")
	for _, url := range urls {
		multiClient.clients = append(multiClient.clients, NewClient(logger, url))
	}
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
