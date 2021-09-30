package env

import (
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
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
	reset := config.GetString("RESET_DATABASE")
	return reset == "true"
}

var whiteList map[string]bool

// InInverseContractWhiteList return true if this contract is inverse
func InInverseContractWhiteList(marginAccountID string) bool {
	if len(whiteList) != 0 {
		return whiteList[marginAccountID]
	}
	whiteList = make(map[string]bool)
	total := config.GetInt("COUNT_INVERSE_CONTRACT_WHITELIST", 0)
	for i := 0; i < total; i++ {
		addr := config.GetString(fmt.Sprintf("INVERSE_CONTRACT_WHITELIST%d", i))
		whiteList[addr] = true
	}
	return whiteList[marginAccountID]
}
