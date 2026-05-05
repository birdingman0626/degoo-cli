package logger

import (
	"fmt"
	"io"
	"os"
	"time"
)

type Logger struct {
	w io.Writer
	f *os.File
}

func New(logPath string) (*Logger, error) {
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Logger{w: io.MultiWriter(os.Stdout, f), f: f}, nil
}

func (l *Logger) Close() error { return l.f.Close() }

func (l *Logger) Info(format string, args ...interface{}) {
	l.write("INFO", format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.write("WARN", format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.write("ERROR", format, args...)
}

func (l *Logger) write(level, format string, args ...interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	if _, err := fmt.Fprintf(l.w, "[%s] [%s] %s\n", ts, level, msg); err != nil {
		fmt.Fprintf(os.Stderr, "logger write error: %v\n", err)
	}
}
