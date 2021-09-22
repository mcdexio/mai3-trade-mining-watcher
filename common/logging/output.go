package logging

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mcdexio/mai3-trade-mining-watcher/cache/cacher"
)

// Shared instances.
var (
	// *multiOutput
	defaultOut = cacher.NewConst(func() interface{} {
		o := multiOutput{}
		if logToStdout {
			o = append(o, Stdout())
		}
		if logToStackdriver {
			o = append(o, Stackdriver())
		}
		if len(o) == 0 {
			fmt.Println("no default logger specified")
		}
		return &o
	})
)

// defaultOutput returns the default output.
func defaultOutput() output {
	return (defaultOut.Get()).(*multiOutput)
}

// outputOpt defines the output option type.
type outputOpt sync.Map

// output defines the log output interface.
type output interface {
	// output outputs the logs.
	output(opt *outputOpt, level level, labelMap labelMap, log string)
}

// newMultiOutput returns a multi output.
func newMultiOutput(outputs ...output) output {
	o := multiOutput(outputs)
	o.expand()
	return &o
}

// multiOutput defines the multiple output.
type multiOutput []output

func (o *multiOutput) expand() {
	expanded := multiOutput{}
	for _, sub := range *o {
		if mo, ok := sub.(*multiOutput); ok {
			mo.expand()
			expanded = append(expanded, *mo...)
			continue
		}
		expanded = append(expanded, sub)
	}
	*o = expanded
}

// output outputs the logs.
func (o *multiOutput) output(opt *outputOpt, level level, labelMap labelMap, log string) {
	l := len(*o)
	if l == 0 {
		return
	} else if l == 1 {
		(*o)[0].output(opt, level, labelMap, log)
		return
	}

	var wg sync.WaitGroup
	for _, out := range *o {
		wg.Add(1)
		go func(o output) {
			defer wg.Done()
			o.output(opt, level, labelMap, log)
		}(out)
	}
	wg.Wait()
}

// removeColor returns a new string with color code removed.
func removeColor(s string) string {
	sb := strings.Builder{}
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' {
			for ; i < len(s) && s[i] != 'm'; i++ {
			}
		} else {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}
