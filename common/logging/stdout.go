package logging

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/cache/cacher"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/utils"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"github.com/ttacon/chalk"
)

var (
	styleMap = map[level]chalk.Style{
		debugLevel:    chalk.ResetColor.NewStyle(),
		infoLevel:     chalk.Green.NewStyle(),
		noticeLevel:   chalk.Cyan.NewStyle(),
		warnLevel:     chalk.Yellow.NewStyle(),
		errorLevel:    chalk.Red.NewStyle(),
		criticalLevel: chalk.Magenta.NewStyle(),
	}

	timeStyle = chalk.ResetColor.NewStyle().WithTextStyle(chalk.Inverse)
	tagStyle  = chalk.ResetColor.NewStyle().WithBackground(chalk.Blue)

	// *stdOutput
	stdout = cacher.NewConst(func() interface{} {
		return newStdOutput()
	})
)

// Stdout returns the stdout output.
//goland:noinspection ALL
func Stdout() output {
	return (stdout.Get()).(*stdOutput)
}

// stdoutOpt returns a new stdout option.
func stdoutOpt() *stdoutOption { return &stdoutOption{} }

// stdoutOption defines the option of stdout.
type stdoutOption struct {
	withColor bool
}

// setWithColor sets if the output with color.
func (o *stdoutOption) setWithColor(c bool) *stdoutOption {
	o.withColor = c
	return o
}

type stdOutput struct {
	writer     io.Writer
	ctx        context.Context
	cancel     context.CancelFunc
	defaultOpt *stdoutOption
	workerChan *utils.UnlimitedChannel
	closeChan  chan struct{}
}

// assertOutputInterface
func _() {
	var _ output = (*stdOutput)(nil)
}

func newStdOutput() *stdOutput {
	opt := stdoutOpt()
	if !env.IsCI() {
		opt = opt.setWithColor(true)
	}
	o := &stdOutput{
		writer:     os.Stdout,
		defaultOpt: opt,
		workerChan: utils.NewUnlimitedChannel(),
		closeChan:  make(chan struct{}),
	}
	o.ctx, o.cancel = context.WithCancel(context.Background())
	go o.work()
	return o
}

func (o *stdOutput) output(opt *outputOpt, level level, labelMap labelMap, log string) {
	var b []byte
	defer func() {
		select {
		case o.workerChan.In() <- b:
		case <-o.ctx.Done():
			fmt.Println("Stdout worker channel closed")
		}
	}()

	stdOpt := o.defaultOpt
	if stdOptIn, ok := (*sync.Map)(opt).Load("stdout"); ok {
		stdOpt = stdOptIn.(*stdoutOption)
	}
	tsRaw := time.Now().Format(utils.TimeFormat)
	svRaw := fmt.Sprintf("%6s", level.String())
	tagRaw := fmt.Sprintf("%16s", labelMap[LabelTag])
	if !stdOpt.withColor {
		if level <= errorLevel {
			log = fmt.Sprintf("%s: %s", labelMap.debugInfo(false), log)
		}
		log = removeColor(log)
		b = []byte(fmt.Sprintf("%s %s %s %s", tsRaw, svRaw, tagRaw, log))
		return
	}

	if level <= errorLevel {
		log = fmt.Sprintf("%s: %s", labelMap.debugInfo(true), log)
	}
	severityStyle := styleMap[level]
	timestamp := timeStyle.Style(tsRaw)
	severity := severityStyle.Style(svRaw)
	tag := tagStyle.Style(tagRaw)

	b = []byte(fmt.Sprintf("%s %s %s %s", timestamp, severity, tag, log))
}

func (o *stdOutput) work() {
	defer func() { close(o.closeChan) }()
	for {
		select {
		case <-o.ctx.Done():
			o.workerChan.Close()
			<-o.workerChan.Done()
			o.flush()
			return
		case b := <-o.workerChan.Out():
			bytes := b.([]byte)
			if len(bytes) <= 0 {
				continue
			}
			_, _ = o.writer.Write(bytes)
		}
	}
}

func (o *stdOutput) flush() {
	for _, bytes := range o.workerChan.Dump() {
		_, _ = o.writer.Write(bytes.([]byte))
	}
}

func (o *stdOutput) close() {
	o.cancel()
	<-o.closeChan
}
