package logging

import (
	"context"

	"cloud.google.com/go/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/cache/cacher"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
)

// *stackdriverOutput
var (
	stackdriverOut = cacher.NewConst(func() interface{} {
		stackdriver, err := newStackdriverOutput(logName)
		if err != nil {
			panic(err)
		}
		return stackdriver
	})
)

// Stackdriver returns the stackdriver output.
//goland:noinspection ALL
func Stackdriver() output {
	return (stackdriverOut.Get()).(*stackdriverOutput)
}

type stackdriverOutput struct {
	client *logging.Client
	logger *logging.Logger
}

// assertOutputInterface
func _() {
	var _ output = (*stackdriverOutput)(nil)
}

func newStackdriverOutput(logname string) (*stackdriverOutput, error) {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, config.GetString("SERVER_PROJECT_ID"))
	if err != nil {
		return nil, err
	}
	// Check if Connection is Valid.
	err = client.Ping(ctx)
	if err != nil {
		return nil, err
	}
	o := &stackdriverOutput{client: client}
	o.refreshLogger(logname)
	return o, nil
}

func (o *stackdriverOutput) refreshLogger(logname string) {
	if o.logger != nil {
		return
	}
	if logname == "" {
		return
	}
	o.logger = o.client.Logger(logname)
}

func (o *stackdriverOutput) output(
	_ *outputOpt, level level, labelMap labelMap, log string) {
	if o.logger == nil {
		return
	}
	o.logger.Log(logging.Entry{
		Severity: level.Severity(),
		Labels:   labelMap,
		Payload:  removeColor(log),
	})
}
