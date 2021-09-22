package errors

import (
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"os"
	"runtime/debug"
)

var logger logging.Logger

// Initialize initializes error reporter.
func Initialize(l logging.Logger) {
	logger = l
}

// Catch is used for logging panic call stack. Catch should be called with defer.
func Catch() {
	if recovered := recover(); recovered != nil {
		logger.Critical("%v\n%s", recovered, string(debug.Stack()))
	}
}

// CatchWithLogger is a panic handler expected to be deferred.
func CatchWithLogger(logger logging.Logger) {
	if recovered := recover(); recovered != nil {
		format := "\x1b[31m%v\n[Stack Trace]\n%s\x1b[m"
		stack := debug.Stack()
		if logger != nil {
			logger.Error(format, recovered, stack)
		} else {
			fmt.Fprintf(os.Stderr, format, recovered, stack)
		}
	}
}
