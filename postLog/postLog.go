package postLog

import (
	"fmt"
	"sync"
	"time"
)

var (
	loggerMutex sync.Mutex
	isDebug     bool
)

const (
	DEBUG = iota
	INFO
	WARNING
	ERROR
	FATAL
)

var levelNames = []string{"DEBUG", "INFO", "WARNING", "ERROR", "FATAL"}
var levelColors = []int{34, 27, 220, 196, 124}

func colorOut_256(s string, foreColor int) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[m", foreColor, s)
}

func getSystemTime() string {
	now := time.Now()
	return now.Format("2006-01-02 15:04:05.000")
}

func SetDebugMode(debug bool) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	isDebug = debug
}

func PostLog(message string, level int) {
	timeNow := getSystemTime()

	idx := level
	if idx < 0 || idx >= len(levelNames) {
		idx = INFO
	}

	loggerMutex.Lock()

	if idx == DEBUG && !isDebug {
		loggerMutex.Unlock()
		return
	}

	levelDisplay := colorOut_256(levelNames[idx], levelColors[idx])
	db := logsDB

	loggerMutex.Unlock()

	fmt.Printf("[%s - %s] %s\n", timeNow, levelDisplay, message)
	insertLogToDB(db, level, message, timeNow)

	if broadcaster != nil {
		broadcaster.AddToHistory(LogMessage{
			Level:     level,
			Content:   message,
			Timestamp: timeNow,
		})
		broadcaster.Broadcast(LogMessage{
			Level:     level,
			Content:   message,
			Timestamp: timeNow,
		})
	}
}

func Debug(message string) {
	PostLog(message, DEBUG)
}

func Info(message string) {
	PostLog(message, INFO)
}

func Warning(message string) {
	PostLog(message, WARNING)
}

func Error(message string) {
	PostLog(message, ERROR)
}

func Fatal(message string) {
	PostLog(message, FATAL)
}
