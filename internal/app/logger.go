package app

import (
	"fmt"
	"os"
	"strings"
)

type LogLevel int

const (
	Debug LogLevel = iota
	Info
	Warn
	Error
	Silent
)

type Logger interface {
	SetLevel(level LogLevel)
	Debug(message string, args ...any)
	Info(message string, args ...any)
	Warn(message string, args ...any)
	Error(message string, args ...any)
}

type SimpleLogger struct {
	Level LogLevel
}

func (s *SimpleLogger) SetLevel(level LogLevel) {
	s.Level = level
}

func ToLogLevel(level string, defaultLevel LogLevel) LogLevel {
	level = strings.ToLower(level)

	switch level {
	case "debug", "d":
		return Debug
	case "info", "i":
		return Info
	case "warn", "warning", "w":
		return Warn
	case "error", "err", "e":
		return Error
	case "silent", "none":
		return Silent
	default:
		return defaultLevel
	}
}

func (s *SimpleLogger) log(level LogLevel, message string, args ...any) {
	if s.Level > level {
		return
	}
	if level < Warn {
		fmt.Printf(message, args...)
	} else {
		fmt.Fprintf(os.Stderr, message, args...)
	}
}

func (s *SimpleLogger) Debug(message string, args ...any) {
	s.log(Debug, message, args...)
}

func (s *SimpleLogger) Info(message string, args ...any) {
	s.log(Info, message, args...)
}

func (s *SimpleLogger) Warn(message string, args ...any) {
	s.log(Warn, message, args...)
}

func (s *SimpleLogger) Error(message string, args ...any) {
	s.log(Error, message, args...)
}
