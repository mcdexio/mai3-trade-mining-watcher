package env

import (
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

// ResetDatabase returns true when reset database.
func ResetDatabase() bool {
	reset := config.GetString("RESET_DATABASE", "false")
	return reset == "true"
}

// BSCChainInclude returns true when include bsc chain into trading mining.
func BSCChainInclude() bool {
	reset := config.GetString("BSC_CHAIN", "false")
	return reset == "true"
}

// ArbRinkebyChainInclude returns true when include arb-rinkeby chain into trading mining.
func ArbRinkebyChainInclude() bool {
	reset := config.GetString("ARB_RINKEBY_CHAIN", "false")
	return reset == "true"
}
