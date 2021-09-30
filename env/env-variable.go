package env

import "github.com/mcdexio/mai3-trade-mining-watcher/common/config"

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
