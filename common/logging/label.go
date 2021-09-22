package logging

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ttacon/chalk"
)

// LabelMap are log Labels
type labelMap map[string]string

// LabelTag Enumeration of Label.
const (
	LabelTag = "tag"
)

const (
	labelProcessID   = "pid"
	labelGoroutineID = "go_id"
	labelFuncName    = "func_name"
	labelFileName    = "file_name"
	labelLineNumber  = "line_number"
)

var (
	goTagColorFunc      = chalk.ResetColor.NewStyle()
	goIDColorFunc       = chalk.ResetColor.NewStyle()
	processTagColorFunc = goTagColorFunc
	processIDColorFunc  = goIDColorFunc
	// gitTagColorFunc        = goTagColorFunc
	// gitCommitHashColorFunc = goIDColorFunc
	funcNameColorFunc = chalk.Cyan.NewStyle()
	fileColorFunc     = chalk.Magenta.NewStyle()
	lineColorFunc     = chalk.Yellow.NewStyle()
)

func (l *labelMap) addDebugInfo(numStackFrame int) {
	(*l)[labelProcessID] = fmt.Sprintf("%d", os.Getpid())

	buffer := make([]byte, 64)
	buffer = buffer[:runtime.Stack(buffer, false)]
	bufList := bytes.Fields(buffer)
	goroutineID := "-1"
	if len(bufList) >= 2 {
		goroutineID = string(bufList[1])
	}
	(*l)[labelGoroutineID] = goroutineID

	funcName := "???"
	pc, file, line, ok := runtime.Caller(numStackFrame)
	if !ok {
		file = "???"
		line = -1
	} else {
		funcName = runtime.FuncForPC(pc).Name()
		file = filepath.Base(file)
	}
	(*l)[labelFuncName] = funcName + "()"
	(*l)[labelFileName] = file
	(*l)[labelLineNumber] = fmt.Sprintf("%d", line)
}

func (l *labelMap) debugInfo(styled bool) string {
	if !styled {
		return fmt.Sprintf(
			"%s%s:%s%s:%s:%s:%s",
			"PID_",
			(*l)[labelProcessID],
			"GoID_",
			(*l)[labelGoroutineID],
			(*l)[labelFuncName],
			(*l)[labelFileName],
			(*l)[labelLineNumber],
		)
	}
	return fmt.Sprintf(
		"%s%s:%s%s:%s:%s:%s",
		processTagColorFunc.Style("PID_"),
		processIDColorFunc.Style((*l)[labelProcessID]),
		goTagColorFunc.Style("GoID_"),
		goIDColorFunc.Style((*l)[labelGoroutineID]),
		funcNameColorFunc.Style((*l)[labelFuncName]),
		fileColorFunc.Style((*l)[labelFileName]),
		lineColorFunc.Style((*l)[labelLineNumber]),
	)
}
