package ux

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ZerologLogger implements Logger using zerolog
type ZerologLogger struct {
	logger zerolog.Logger
}

func NewZerologLogger() Logger {
	return &ZerologLogger{logger: log.Logger}
}

func NewZerologLoggerWithLogger(logger zerolog.Logger) Logger {
	return &ZerologLogger{logger: logger}
}

func (l *ZerologLogger) Info(msg string, fields ...LogField) {
	event := l.logger.Info()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

func (l *ZerologLogger) Warn(msg string, fields ...LogField) {
	event := l.logger.Warn()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

func (l *ZerologLogger) Error(msg string, fields ...LogField) {
	event := l.logger.Error()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

func (l *ZerologLogger) Debug(msg string, fields ...LogField) {
	event := l.logger.Debug()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}
