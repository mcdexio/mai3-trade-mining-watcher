package env

import (
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"strings"
)

// IsCI returns true if we are in CI mode.
func IsCI() bool {
	ci := config.GetString("CI", "false")
	return ci == "true"
}

// LogColor returns true if log color on
func LogColor() bool {
	color := config.GetString("LOG_COLOR")
	return color == "true"
}

// ResetDatabase returns true if want reset database.
func ResetDatabase() bool {
	reset := config.GetString("RESET_DATABASE", "false")
	return reset == "true"
}

var btcWhiteList map[string]string
var ethWhiteList map[string]string

// InBTCInverseContractWhiteList return true, quote if this contract is inverse
func InBTCInverseContractWhiteList(marginAccountID string) (bool, string) {
	if len(btcWhiteList) != 0 {
		if quote, match := btcWhiteList[marginAccountID]; match {
			return true, quote
		}
		return false, ""
	}
	btcWhiteList = make(map[string]string)
	total := config.GetInt("BTC_COUNT_INVERSE_CONTRACT_WHITELIST", 0)
	for i := 0; i < total; i++ {
		addr := config.GetString(fmt.Sprintf("BTC_INVERSE_CONTRACT_WHITELIST%d", i))
		if i == 0 {
			btcWhiteList[addr] = "USD"
		} else if i == 1 {
			btcWhiteList[addr] = "USD"
		}
	}
	if quote, match := btcWhiteList[marginAccountID]; match {
		return true, quote
	}
	return false, ""
}

// InETHInverseContractWhiteList return true, quote if this contract is inverse
func InETHInverseContractWhiteList(marginAccountID string) (bool, string) {
	if len(ethWhiteList) != 0 {
		if quote, match := ethWhiteList[marginAccountID]; match {
			return true, quote
		}
		return false, ""
	}
	ethWhiteList = make(map[string]string)
	total := config.GetInt("ETH_COUNT_INVERSE_CONTRACT_WHITELIST", 0)
	for i := 0; i < total; i++ {
		addr := config.GetString(fmt.Sprintf("ETH_INVERSE_CONTRACT_WHITELIST%d", i))
		if i == 0 {
			ethWhiteList[addr] = "USD"
		} else if i == 1 {
			ethWhiteList[addr] = "USD"
		} else if i == 2 {
			ethWhiteList[addr] = "BTC"
		}
	}
	if quote, match := ethWhiteList[marginAccountID]; match {
		return true, quote
	}
	return false, ""
}

// GetPerpIDWithUSDBased get perpetual id depend on symbol and network.
func GetPerpIDWithUSDBased(symbol string, networkIndex int) (string, error) {
	// TODO(champFu): refactor
	// bsc == 0 networkIndex
	// arb-rinkeby == 1 networkIndex

	symbol = strings.ToLower(symbol)
	if networkIndex == 0 {
		if symbol == "btc" {
			return "0xdb282bbace4e375ff2901b84aceb33016d0d663d-0", nil
		} else if symbol == "eth" {
			return "0xdb282bbace4e375ff2901b84aceb33016d0d663d-1", nil
		} else if symbol == "bnb" {
			return "0xdb282bbace4e375ff2901b84aceb33016d0d663d-2", nil
		}
		return "", fmt.Errorf("fail to get perpetualID of symbol %s", symbol)
	} else if networkIndex == 1 {
		if symbol == "eth" {
			return "0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0", nil // perpIndex = 0 is ETH
		} else if symbol == "btc" {
			return "0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-1", nil // perpIndex = 1 is BTC
		}
		return "", fmt.Errorf("fail to get perpetualID of symbol %s", symbol)
	}
	return "", fmt.Errorf("fail to get perpetualID of symbol %s", symbol)
}
