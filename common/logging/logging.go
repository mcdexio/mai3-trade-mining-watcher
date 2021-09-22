package logging

import "github.com/mcdexio/mai3-trade-mining-watcher/common/config"

// Variables only are used in logging package.
var (
	logToStdout      = config.GetBool("SERVER_LOG_TO_STDOUT", true)
	logToStackdriver = config.GetBool("SERVER_LOG_TO_STACKDRIVER", false)

	hostName, logName string
)

// Initialize initializes the logging package.
func Initialize(logname string) {
	logName = logname
	hostName = config.GetString("HOSTNAME", "localhost")

	if !logToStackdriver {
		return
	}

	Stackdriver().(*stackdriverOutput).refreshLogger(logName)
}

// Finalize finalizes the logger module.
func Finalize() {
	// flush stdout.
	if stdout.IsLoaded() {
		Stdout().(*stdOutput).close()
		stdout.Clear()
	}

	// flush stackdriver out.
	if stackdriverOut.IsLoaded() {
		err := Stackdriver().(*stackdriverOutput).client.Close()
		if err != nil {
			panic(err)
		}
		stackdriverOut.Clear()
	}
}
