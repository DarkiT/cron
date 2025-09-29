package cron

import (
	"context"
	"fmt"
	"runtime"
)

// PanicHandler 定义panic处理器接口
type PanicHandler interface {
	HandlePanic(taskID string, panicValue interface{}, stack []byte)
}

// DefaultPanicHandler 默认的panic处理器
type DefaultPanicHandler struct {
	logger Logger
}

// NewDefaultPanicHandler 创建默认panic处理器
func NewDefaultPanicHandler(logger Logger) *DefaultPanicHandler {
	if logger == nil {
		logger = NewDefaultLogger()
	}
	return &DefaultPanicHandler{logger: logger}
}

// HandlePanic 默认的panic处理实现
func (h *DefaultPanicHandler) HandlePanic(taskID string, panicValue interface{}, stack []byte) {
	if h.logger != nil {
		h.logger.Errorf("PANIC in task %s: %v\nStack trace:\n%s", taskID, panicValue, stack)
	}
}

// SafeCall 安全调用函数，捕获并处理panic
func SafeCall(taskID string, fn func(), handler PanicHandler) (recovered bool) {
	defer func() {
		if r := recover(); r != nil {
			recovered = true

			// 获取堆栈信息
			stack := make([]byte, 4096)
			n := runtime.Stack(stack, false)

			if handler != nil {
				handler.HandlePanic(taskID, r, stack[:n])
			} else {
				// 使用默认处理器
				defaultHandler := NewDefaultPanicHandler(nil)
				defaultHandler.HandlePanic(taskID, r, stack[:n])
			}
		}
	}()

	fn()
	return false
}

// WithPanicHandler 设置panic处理器的选项
func WithPanicHandler(handler PanicHandler) Option {
	return func(c *Cron) {
		c.panicHandler = handler
	}
}

// RecoveryJob 带异常恢复的Job包装器
type RecoveryJob struct {
	originalJob  Job
	taskID       string
	panicHandler PanicHandler
}

// Name 实现Job接口，返回原始任务的名称
func (r *RecoveryJob) Name() string {
	return r.originalJob.Name()
}

// Run 实现Job接口，添加异常捕获
func (r *RecoveryJob) Run(ctx context.Context) error {
	var jobErr error

	recovered := SafeCall(r.taskID, func() {
		jobErr = r.originalJob.Run(ctx)
	}, r.panicHandler)

	if recovered {
		return fmt.Errorf("task %s recovered from panic", r.taskID)
	}

	return jobErr
}
