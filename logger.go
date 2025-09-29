package cron

import (
	"fmt"
	"log/slog"
)

// DefaultLogger 默认日志实现，使用log/slog
type DefaultLogger struct {
	logger *slog.Logger
}

// NewDefaultLogger 创建默认日志实现
func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		logger: slog.Default(),
	}
}

// Debugf 输出调试日志
func (l *DefaultLogger) Debugf(format string, args ...any) {
	if l.logger != nil {
		l.logger.Debug(fmt.Sprintf(format, args...))
	}
}

// Infof 输出信息日志
func (l *DefaultLogger) Infof(format string, args ...any) {
	if l.logger != nil {
		l.logger.Info(fmt.Sprintf(format, args...))
	}
}

// Warnf 输出警告日志
func (l *DefaultLogger) Warnf(format string, args ...any) {
	if l.logger != nil {
		l.logger.Warn(fmt.Sprintf(format, args...))
	}
}

// Errorf 输出错误日志
func (l *DefaultLogger) Errorf(format string, args ...any) {
	if l.logger != nil {
		l.logger.Error(fmt.Sprintf(format, args...))
	}
}

// NoOpLogger 空日志实现，不输出任何内容
type NoOpLogger struct{}

// Debugf 空实现
func (l *NoOpLogger) Debugf(format string, args ...any) {}

// Infof 空实现
func (l *NoOpLogger) Infof(format string, args ...any) {}

// Warnf 空实现
func (l *NoOpLogger) Warnf(format string, args ...any) {}

// Errorf 空实现
func (l *NoOpLogger) Errorf(format string, args ...any) {}
