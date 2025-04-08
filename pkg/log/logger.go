package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// Logger is a simple logger with different severity levels
type Logger struct {
	name   string
	level  LogLevel
	writer io.Writer
}

// NewLogger creates a new logger with the specified name
func NewLogger(name string) *Logger {
	level := InfoLevel
	levelStr := os.Getenv("INPUT_LOG_LEVEL")
	if levelStr != "" {
		switch strings.ToLower(levelStr) {
		case "debug":
			level = DebugLevel
		case "info":
			level = InfoLevel
		case "warn", "warning":
			level = WarnLevel
		case "error":
			level = ErrorLevel
		}
	}

	return &Logger{
		name:   name,
		level:  level,
		writer: os.Stdout,
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...interface{}) {
	if l.level <= DebugLevel {
		l.log("DEBUG", msg, args...)
	}
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...interface{}) {
	if l.level <= InfoLevel {
		l.log("INFO", msg, args...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...interface{}) {
	if l.level <= WarnLevel {
		l.log("WARN", msg, args...)
	}
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...interface{}) {
	if l.level <= ErrorLevel {
		l.log("ERROR", msg, args...)
	}
}

// log formats and writes a log message
func (l *Logger) log(level, msg string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Format the key-value pairs
	var kvPairs string
	if len(args) > 0 && len(args)%2 == 0 {
		pairs := make([]string, 0, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			key, val := args[i], args[i+1]
			pairs = append(pairs, fmt.Sprintf("%v=%v", key, val))
		}
		if len(pairs) > 0 {
			kvPairs = " " + strings.Join(pairs, " ")
		}
	}

	// Special handling for GitHub Actions workflow commands
	if level == "ERROR" {
		// Use GitHub Actions error annotation
		fmt.Fprintf(l.writer, "::error::%s\n", msg)
	}

	logLine := fmt.Sprintf("%s [%s] %s: %s%s\n", timestamp, level, l.name, msg, kvPairs)
	fmt.Fprint(l.writer, logLine)
}

// WithField returns a new logger with a field added to each log message
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		name:   fmt.Sprintf("%s.%s=%v", l.name, key, value),
		level:  l.level,
		writer: l.writer,
	}
}
