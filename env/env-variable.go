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

// MultiChainEpochStart return epoch number which multi-chain start
func MultiChainEpochStart() int64 {
	epoch := config.GetInt64("MULTI_CHAIN_EPOCH_START", 2)
	return epoch
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

// ArbOneChainInclude returns true when include arbitrum chain into trading mining.
func ArbOneChainInclude() bool {
	reset := config.GetString("ARB_ONE_CHAIN", "false")
	return reset == "true"
}
