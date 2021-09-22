package env

import "github.com/mcdexio/mai3-trade-mining-watcher/common/config"

// IsCI returns true if we are in CI mode.
func IsCI() bool {
	ci := config.GetString("CI", "false")
	return ci == "true"
}
