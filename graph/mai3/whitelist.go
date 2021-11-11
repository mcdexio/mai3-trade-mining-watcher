package mai3

import "github.com/mcdexio/mai3-trade-mining-watcher/common/logging"

type Whitelist struct {
	logger logging.Logger
	list   map[string]string
}

// NewWhiteList makes sure order of contract address.
// index = 0 should be USD, index = 1 should be BTC, index = 2 should be ETH.
func NewWhiteList(logger logging.Logger, contractAddr ...string) *Whitelist {
	w := Whitelist{
		list: make(map[string]string, 0),
	}
	for i, addr := range contractAddr {
		if addr == "" {
			logger.Error("index %d of contractAddr is empty", i)
			return nil
		}
		if i == 0 {
			w.list[addr] = "USD"
		} else if i == 1 {
			w.list[addr] = "BTC"
		} else if i == 2 {
			w.list[addr] = "ETH"
		}
	}
	return &w
}

func (w *Whitelist) InInverseContractWhiteList(perpID string) (bool, string) {
	if base, match := w.list[perpID]; match {
		return true, base
	} else {
		return false, ""
	}
}
