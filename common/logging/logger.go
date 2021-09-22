package logging

import (
	"fmt"
	"os"
	"sync"
)

// opt returns a new LogOption.
func opt() *logOption { return &logOption{0, infoLevel, (*outputOpt)(&sync.Map{})} }

// defaultOpt returns a default LogOption.
func defaultOpt() *logOption { return opt() }

// logOption defines the log option struct.
type logOption struct {
	stackNum  int
	level     level
	outputOpt *outputOpt
}

// setStackNum sets the stack number of logOption.
func (o *logOption) setStackNum(num int) *logOption {
	o.stackNum = num
	if o.stackNum < 0 {
		o.stackNum = 0
	}
	return o
}

// setStackNumDelta sets the stack number of logOption with delta.
//nolint:unused
func (o *logOption) setStackNumDelta(d int) *logOption {
	o.stackNum += d
	if o.stackNum < 0 {
		o.stackNum = 0
	}
	return o
}

// setLevel sets the level of logOption.
func (o *logOption) setLevel(level level) *logOption {
	o.level = level
	return o
}

// OutputOpt sets the output option of logOption.
func (o *logOption) OutputOpt(key string, opt interface{}) *logOption {
	(*sync.Map)(o.outputOpt).Store(key, opt)
	return o
}

// clone returns a clone of logOption.
func (o *logOption) clone() *logOption {
	newOpt := opt().
		setStackNum(o.stackNum).
		setLevel(o.level)
	(*sync.Map)(o.outputOpt).Range(func(k, v interface{}) bool {
		newOpt.OutputOpt(k.(string), v)
		return true
	})
	return newOpt
}

// Logger defines the logger interface.
type Logger interface {
	CloneLogger() Logger

	AppendOutput(output)

	rangeLabelMap(fn func(key, value string))
	getLabelValue(label string) (value string, found bool)
	SetLabel(label string, value string)
	deleteLabel(label string)
	clearLabelMap()

	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Notice(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Critical(format string, args ...interface{})

	log(opt *logOption, format string, args ...interface{})
}

// assertLoggerInterface
func _() {
	var _ Logger = (*logger)(nil)
}

// logger defines the logger.
type logger struct {
	sync.RWMutex

	labelMap       labelMap
	thresholdLevel level
	output         output

	defaultOpt *logOption
}

// loggerOpt defines the logger options.
type loggerOpt struct {
	labelMap       labelMap
	thresholdLevel level
	output         output
}

// NewLogger returns a new logger.
func NewLogger(opt ...*loggerOpt) Logger {
	return NewLoggerTag("", opt...)
}

// NewLoggerTag returns a new logger.
func NewLoggerTag(tag string, opt ...*loggerOpt) Logger {
	// init new logger instance.
	logger := &logger{
		labelMap:       labelMap{LabelTag: tag},
		thresholdLevel: defaultThresholdLevel(),
		output:         defaultOutput(),
		defaultOpt:     defaultOpt(),
	}

	// config with opt if specified.
	if len(opt) == 1 && opt[0] != nil {
		if len(opt[0].labelMap) > 0 {
			for k, v := range opt[0].labelMap {
				logger.labelMap[k] = v
			}
		}
		if opt[0].thresholdLevel.IsValid() {
			logger.thresholdLevel = opt[0].thresholdLevel
		}
		if opt[0].output != nil {
			logger.output = opt[0].output
		}
	}

	// check if level is valid.
	if !logger.thresholdLevel.IsValid() {
		panic(fmt.Sprintf("invalid log threshold level (%d, %d), [%d]",
			firstLevel, lastLevel, logger.thresholdLevel))
	}
	return logger
}

// CloneLogger returns a cloned logger.
func (l *logger) CloneLogger() Logger {
	l.RLock()
	defer l.RUnlock()
	m := labelMap{}
	for key, value := range l.labelMap {
		m[key] = value
	}
	return &logger{
		labelMap:       m,
		thresholdLevel: l.thresholdLevel,
		output:         l.output,
		defaultOpt:     l.defaultOpt.clone(),
	}
}

// AppendOutput appends a output.
func (l *logger) AppendOutput(o output) {
	l.output = newMultiOutput(l.output, o)
}

// rangeLabelMap ranges labelMap data.
func (l *logger) rangeLabelMap(fn func(key, value string)) {
	l.RLock()
	defer l.RUnlock()
	for key, value := range l.labelMap {
		fn(key, value)
	}
}

// getLabelValue returns labelMap.
func (l *logger) getLabelValue(label string) (value string, found bool) {
	l.RLock()
	defer l.RUnlock()
	value, found = l.labelMap[label]
	return
}

// SetLabel returns labelMap.
func (l *logger) SetLabel(label string, value string) {
	l.Lock()
	defer l.Unlock()
	l.labelMap[label] = value
}

// deleteLabel deletes Label.
func (l *logger) deleteLabel(label string) {
	l.Lock()
	defer l.Unlock()
	delete(l.labelMap, label)
}

// clearLabelMap returns labelMap.
func (l *logger) clearLabelMap() {
	l.Lock()
	defer l.Unlock()
	l.labelMap = labelMap{}
}

// Debug - logger level of dubug
func (l *logger) Debug(format string, args ...interface{}) {
	l.print(l.defaultOpt.outputOpt, 3, debugLevel, format, args...)
}

// Info - logger level of info
func (l *logger) Info(format string, args ...interface{}) {
	l.print(l.defaultOpt.outputOpt, 3, infoLevel, format, args...)
}

// Notice - logger level of notice
func (l *logger) Notice(format string, args ...interface{}) {
	l.print(l.defaultOpt.outputOpt, 3, noticeLevel, format, args...)
}

// Warn - logger level of warn
func (l *logger) Warn(format string, args ...interface{}) {
	l.print(l.defaultOpt.outputOpt, 3, warnLevel, format, args...)
}

// Error - logger level of error
func (l *logger) Error(format string, args ...interface{}) {
	l.print(l.defaultOpt.outputOpt, 3, errorLevel, format, args...)
}

// Critical - logger level of error
func (l *logger) Critical(format string, args ...interface{}) {
	l.print(l.defaultOpt.outputOpt, 3, criticalLevel, format, args...)
}

// log - logger level of dubug
func (l *logger) log(opt *logOption, format string, args ...interface{}) {
	if opt == nil {
		opt = l.defaultOpt
	}
	l.print(opt.outputOpt, opt.stackNum+3, opt.level, format, args...)
}

func (l *logger) print(opt *outputOpt, numStackFrame int, level level,
	format string, args ...interface{}) {
	defer func() {
		if level <= criticalLevel {
			Finalize()
			os.Exit(1)
		}
	}()
	if level > l.thresholdLevel {
		return
	}
	l.RLock()
	m := labelMap{}
	for key, value := range l.labelMap {
		m[key] = value
	}
	l.RUnlock()

	// Setup default tags.
	m["pod"] = hostName
	if t := m[LabelTag]; t == "" {
		m[LabelTag] = hostName
	}

	if level <= errorLevel {
		m.addDebugInfo(numStackFrame)
	}

	l.output.output(opt, level, m, fmt.Sprintf(format, args...)+"\n")
}
