package syncer

import (
	"github.com/mcdexio/mai3-trade-mining-watcher/graph/block"
)

type MockBlockGraph1 struct{}

func (mockBlock *MockBlockGraph1) GetBlockNumberWithTS(timestamp int64) (int64, error) {
	// 60 second for 1 block:
	// timestamp 0~59 return 0, timestamp 60~119 return 1
	return timestamp / 60, nil
}

func NewMockBlockGraph1() *MockBlockGraph1 {
	return &MockBlockGraph1{}
}

type MockMultiBlockGraphs struct {
	clients []block.BlockInterface
}

func (mockMultiBlocks *MockMultiBlockGraphs) GetMultiBlockNumberWithTS(timestamp int64) ([]int64, error) {
	var ret []int64
	for _, blockGraph := range mockMultiBlocks.clients {
		bn, err := blockGraph.GetBlockNumberWithTS(timestamp)
		if err != nil {
			return nil, err
		}
		ret = append(ret, bn)
	}
	return ret, nil
}

func NewMockMultiBlockGraphsOneChain() *MockMultiBlockGraphs {
	multiBlockClients := MockMultiBlockGraphs{
		clients: make([]block.BlockInterface, 0),
	}
	multiBlockClients.clients = append(multiBlockClients.clients, NewMockBlockGraph1())
	return &multiBlockClients
}

func NewMockMultiBlockGraphsMultiChain() *MockMultiBlockGraphs {
	multiBlockClients := MockMultiBlockGraphs{
		clients: make([]block.BlockInterface, 0),
	}
	multiBlockClients.clients = append(multiBlockClients.clients, NewMockBlockGraph1())
	multiBlockClients.clients = append(multiBlockClients.clients, NewMockBlockGraph1())
	return &multiBlockClients
}
