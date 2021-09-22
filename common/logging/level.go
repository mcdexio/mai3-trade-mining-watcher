package logging

import (
	"cloud.google.com/go/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
)

// level of logger
type level int

// defaultThresholdLevel returns the default log level.
func defaultThresholdLevel() level {
	l := level(int(config.GetInt64("SERVER_LOGLEVEL", 6)))
	return l
}

// Log / Severity Levels
const (
	firstLevel level = iota
	criticalLevel
	errorLevel
	warnLevel
	noticeLevel
	infoLevel
	debugLevel
	lastLevel
)

// IsValid returns if the l is valid.
func (l level) IsValid() bool {
	return l < lastLevel && l > firstLevel
}

// String returns the string description of l.
func (l level) String() string {
	var levelName = []string{
		"",
		" CRIT",
		"ERROR",
		" WARN",
		" NOTE",
		" INFO",
		"DEBUG",
		"",
	}
	return levelName[l]
}

// Severity returns the severity.
func (l level) Severity() logging.Severity {
	var levelSeverity = []logging.Severity{
		-1,
		logging.Critical,
		logging.Error,
		logging.Warning,
		logging.Notice,
		logging.Info,
		logging.Debug,
		-1,
	}
	return levelSeverity[l]
}
