package cron

import (
	"context"
	"time"
)

// PanicHandler 定义处理 panic 的函数类型
type PanicHandler interface {
	Handle(jobName string, err interface{})
}

// Option 定义了 Crontab 的配置选项
type Option func(*Crontab)

// WithPanicHandler 设置 panic 处理器
// 参数：
//   - handler: 实现了 PanicHandler 接口的处理器
//
// 返回：
//   - Option: 返回配置选项函数
func WithPanicHandler(handler PanicHandler) Option {
	return func(c *Crontab) {
		if c.scheduler != nil {
			c.scheduler.panicHandler = handler
			// 自动为所有任务启用 TryCatch
			c.scheduler.mu.Lock()
			for _, job := range c.scheduler.tasks {
				job.tryCatch = true
			}
			c.scheduler.mu.Unlock()
		}
	}
}

// WithLogger 设置自定义日志记录器
func WithLogger(logger Logger) Option {
	return func(c *Crontab) {
		if logger != nil {
			SetLogger(logger)
		}
	}
}

// JobOption 定义任务选项，用于配置任务模型的函数类型
type JobOption func(*jobModel)

// WithAsync 设置任务是否异步执行
// 参数：
//   - async: true 表示异步执行，false 表示同步执行
//
// 返回：
//   - JobOption: 返回一个任务选项函数
func WithAsync(async bool) JobOption {
	return func(j *jobModel) {
		j.async = async
	}
}

// WithTryCatch 设置任务是否启用 panic 捕获
// 参数：
//   - tryCatch: true 表示启用 panic 捕获，false 表示禁用
//
// 返回：
//   - JobOption: 返回一个任务选项函数
func WithTryCatch(tryCatch bool) JobOption {
	return func(j *jobModel) {
		j.tryCatch = tryCatch
	}
}

// WithTimeout 设置任务的超时时间
// 参数：
//   - timeout: 超时时间，如果为 0 则表示不设置超时
//
// 返回：
//   - JobOption: 返回一个任务选项函数
func WithTimeout(timeout time.Duration) JobOption {
	return func(j *jobModel) {
		j.timeout = timeout
	}
}

// WithMaxConcurrent 设置任务的最大并发数
// 参数：
//   - maxConcurrent: 最大并发数，如果为 0 则表示不限制
//
// 返回：
//   - JobOption: 返回一个任务选项函数
func WithMaxConcurrent(maxConcurrent int) JobOption {
	return func(j *jobModel) {
		j.maxConcurrent = maxConcurrent
		// 初始化队列
		if maxConcurrent > 0 {
			j.queue = make(chan struct{}, maxConcurrent)
		}
	}
}

// WithContextFunc 设置支持上下文的执行函数
// 参数：
//   - fn: 接收上下文的执行函数
//
// 返回：
//   - JobOption: 返回一个任务选项函数
func WithContextFunc(fn func(ctx context.Context)) JobOption {
	return func(j *jobModel) {
		if fn != nil {
			j.doWithContext = fn
			j.do = func() {
				// 创建可取消的上下文
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// 如果设置了超时，添加超时控制
				if j.timeout > 0 {
					var ctxWithTimeout context.Context
					ctxWithTimeout, cancel = context.WithTimeout(ctx, j.timeout)
					defer cancel()
					ctx = ctxWithTimeout
				}

				fn(ctx)
			}
		}
	}
}

// Logger 定义了日志接口
type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}
