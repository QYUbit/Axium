package slogadapter

import (
	"log/slog"
)

type Adapter struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

func (a *Adapter) Info(msg string, keysAndValues ...any) {
	a.logger.Info(msg, keysAndValues...)
}

func (a *Adapter) Error(msg string, keysAndValues ...any) {
	a.logger.Error(msg, keysAndValues...)
}

func (a *Adapter) Debug(msg string, keysAndValues ...any) {
	a.logger.Debug(msg, keysAndValues...)
}

func (a *Adapter) Warn(msg string, keysAndValues ...any) {
	a.logger.Warn(msg, keysAndValues...)
}
