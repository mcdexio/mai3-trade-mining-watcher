package block

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"golang.org/x/sync/errgroup"
)

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
	g := new(errgroup.Group)

	bns := make([]int64, len(c.clients))
	for i, blockGraph := range c.clients {
		i, blockGraph := i, blockGraph
		g.Go(func() error {
			result, err := blockGraph.GetBlockNumberWithTS(timestamp)
			if err != nil {
				return err
			}
			bns[i] = result
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return bns, nil
}
