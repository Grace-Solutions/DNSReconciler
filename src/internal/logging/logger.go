package logging

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type Level string

const (
	LevelTrace       Level = "Trace"
	LevelDebug       Level = "Debug"
	LevelInformation Level = "Information"
	LevelWarning     Level = "Warning"
	LevelError       Level = "Error"
	LevelCritical    Level = "Critical"
)

var levelOrder = map[Level]int{
	LevelTrace: 0, LevelDebug: 1, LevelInformation: 2,
	LevelWarning: 3, LevelError: 4, LevelCritical: 5,
}

type Logger struct {
	mu         sync.Mutex
	writer     io.Writer
	fileWriter *RotatingFileWriter
	level      Level
}

func New(writer io.Writer, level Level) *Logger {
	if _, ok := levelOrder[level]; !ok {
		level = LevelInformation
	}
	return &Logger{writer: writer, level: level}
}

// AttachFileWriter adds a rotating file writer so log output goes to both
// the original writer (typically stderr) and the log file.
func (l *Logger) AttachFileWriter(fw *RotatingFileWriter) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fileWriter = fw
}

// CloseFileWriter closes the attached rotating file writer, if any.
func (l *Logger) CloseFileWriter() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fileWriter != nil {
		_ = l.fileWriter.Close()
		l.fileWriter = nil
	}
}

func ParseLevel(value string) Level {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "trace":
		return LevelTrace
	case "debug":
		return LevelDebug
	case "warning":
		return LevelWarning
	case "error":
		return LevelError
	case "critical":
		return LevelCritical
	default:
		return LevelInformation
	}
}

func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := levelOrder[level]; ok {
		l.level = level
	}
}

func (l *Logger) Trace(message string)       { l.log(LevelTrace, message) }
func (l *Logger) Debug(message string)       { l.log(LevelDebug, message) }
func (l *Logger) Information(message string) { l.log(LevelInformation, message) }
func (l *Logger) Warning(message string)     { l.log(LevelWarning, message) }
func (l *Logger) Error(message string)       { l.log(LevelError, message) }
func (l *Logger) Critical(message string)    { l.log(LevelCritical, message) }

func (l *Logger) log(level Level, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if levelOrder[level] < levelOrder[l.level] {
		return
	}
	formatted := fmt.Sprintf("[%s] - [%s] - %s\n",
		time.Now().UTC().Format("2006-01-02 15:04:05.000Z"),
		level,
		strings.TrimSpace(message),
	)
	_, _ = io.WriteString(l.writer, formatted)
	if l.fileWriter != nil {
		_, _ = l.fileWriter.Write([]byte(formatted))
	}
}