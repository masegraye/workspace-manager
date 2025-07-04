package ux

import (
	"fmt"
	"log"
	"strings"
)

// StdLogger implements Logger using standard library log package
type StdLogger struct{}

func NewStdLogger() Logger {
	return &StdLogger{}
}

func (l *StdLogger) Info(msg string, fields ...LogField) {
	log.Printf("INFO: %s%s", msg, l.formatFields(fields))
}

func (l *StdLogger) Warn(msg string, fields ...LogField) {
	log.Printf("WARN: %s%s", msg, l.formatFields(fields))
}

func (l *StdLogger) Error(msg string, fields ...LogField) {
	log.Printf("ERROR: %s%s", msg, l.formatFields(fields))
}

func (l *StdLogger) Debug(msg string, fields ...LogField) {
	log.Printf("DEBUG: %s%s", msg, l.formatFields(fields))
}

func (l *StdLogger) formatFields(fields []LogField) string {
	if len(fields) == 0 {
		return ""
	}

	var parts []string
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", field.Key, field.Value))
	}

	return " [" + strings.Join(parts, " ") + "]"
}
