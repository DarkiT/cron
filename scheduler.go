package cron

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/darkit/cron/internal/parser"
)

var (
	ErrNotFoundJob     = errors.New("job not found")
	ErrAlreadyRegister = errors.New("job already exists")
	ErrJobDOFuncNil    = errors.New("callback function is nil")
	ErrInvalidSpec     = errors.New("invalid cron specification")
)

// 定义日志接口适配器
type loggerAdapter struct {
	logger *slog.Logger
}

func (l *loggerAdapter) Debug(msg string) {
	l.logger.Debug(msg)
}

func (l *loggerAdapter) Info(msg string) {
	l.logger.Info(msg)
}

func (l *loggerAdapter) Warn(msg string) {
	l.logger.Warn(msg)
}

func (l *loggerAdapter) Error(msg string) {
	l.logger.Error(msg)
}

// 设置默认的日志记录器
var defaultLogger Logger = &loggerAdapter{logger: slog.Default()}

// SetLogger 允许用户设置自定义日志记录器
func SetLogger(logger Logger) {
	if logger != nil {
		defaultLogger = logger
	}
}

// NewCronScheduler 创建一个新的 CronScheduler
func NewCronScheduler() *cronScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &cronScheduler{
		tasks:  make(map[string]*jobModel),
		ctx:    ctx,
		cancel: cancel,
		wg:     &sync.WaitGroup{},
		once:   &sync.Once{},
		mu:     sync.RWMutex{},
	}
}

// CronScheduler 定义了定时任务调度器的核心功能
type cronScheduler struct {
	tasks        map[string]*jobModel // 任务列表
	ctx          context.Context      // 上下文
	cancel       context.CancelFunc   // 取消上下文的函数
	wg           *sync.WaitGroup      // 等待组
	once         *sync.Once           // 确保任务只启动一次
	panicHandler PanicHandler         // panic 处理器
	mu           sync.RWMutex         // 读写锁
}

// Start - 启动所有任务，仅执行一次
func (c *cronScheduler) Start() {
	c.once.Do(func() {
		for _, job := range c.tasks {
			c.wg.Add(1)
			job.runLoop(c.wg)
		}
	})
}

// Stop - 停止所有任务
func (c *cronScheduler) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for name, job := range c.tasks {
		job.kill()
		delete(c.tasks, name)
	}
	c.cancel()
}

// Wait - 等待所有任务退出
func (c *cronScheduler) Wait() {
	c.wg.Wait()
}

// WaitStop - 等待调度器停止
func (c *cronScheduler) WaitStop() {
	<-c.ctx.Done()
}

// Register 注册一个新任务，但不会立即启动
// 参数：
//   - name: 任务名称
//   - model: 任务模型
//
// 返回：
//   - error: 如果注册失败则返回错误
func (c *cronScheduler) Register(name string, model *jobModel) error {
	model.name = name
	model.scheduler = c
	return c.reset(name, model, true, false)
}

// UpdateJobModel - 停止旧任务，更新指定名称的任务
func (c *cronScheduler) UpdateJobModel(name string, model *jobModel) error {
	return c.reset(name, model, false, true)
}

// DynamicRegister - 运行时动态添加任务，任务会自动启动
func (c *cronScheduler) DynamicRegister(name string, model *jobModel) error {
	return c.reset(name, model, false, true)
}

// UnRegister - 停止并删除任务
func (c *cronScheduler) UnRegister(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldModel, ok := c.tasks[name]
	if !ok {
		return ErrNotFoundJob
	}

	oldModel.kill()
	delete(c.tasks, name)
	return nil
}

// StopService - 停止指定名称的任务
func (c *cronScheduler) StopService(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	job, ok := c.tasks[name]
	if !ok {
		return
	}

	job.kill()
	delete(c.tasks, name)
}

// StopServicePrefix - 停止所有符合前缀的任务
func (c *cronScheduler) StopServicePrefix(regex string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for name, job := range c.tasks {
		if !strings.HasPrefix(name, regex) {
			continue
		}

		job.kill()
		delete(c.tasks, name)
	}
}

// GetServiceCron - 获取指定名称的任务
func (c *cronScheduler) GetServiceCron(name string) (*jobModel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	oldModel, ok := c.tasks[name]
	if !ok {
		return nil, ErrNotFoundJob
	}

	return oldModel, nil
}

// GetServiceStatus - 获取指定任务的运行状态
func (c *cronScheduler) GetServiceStatus(name string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	oldModel, ok := c.tasks[name]
	if !ok {
		return false, ErrNotFoundJob
	}

	return oldModel.running > 0, nil
}

// SetPanicHandler - 设置 panic 处理器
func (c *cronScheduler) SetPanicHandler(handler PanicHandler) {
	c.panicHandler = handler
}

// reset - 重置任务
func (c *cronScheduler) reset(name string, model *jobModel, denyReplace, autoStart bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 验证任务模型
	err := model.validate()
	if err != nil {
		return err
	}

	cctx, cancel := context.WithCancel(c.ctx)
	model.ctx = cctx
	model.cancel = cancel
	model.name = name

	oldModel, ok := c.tasks[name]
	if denyReplace && ok {
		return ErrAlreadyRegister
	}

	if ok {
		oldModel.kill()
	}

	c.tasks[name] = model
	if autoStart {
		c.wg.Add(1)
		go c.tasks[name].runLoop(c.wg)
	}

	return nil
}

// NewJobModel 创建任务模型
func NewJobModel(spec string, fn func(), opts ...JobOption) (*jobModel, error) {
	// 解析cron表达式并缓存
	schedule, err := parser.NewParser(parser.Second | parser.Minute | parser.Hour | parser.Dom | parser.Month | parser.Dow).Parse(spec)
	if err != nil {
		return nil, ErrInvalidSpec
	}

	job := &jobModel{
		spec:     spec,
		do:       fn,
		running:  1,
		schedule: schedule, // 存储解析后的表达式
	}

	// 应用选项
	for _, opt := range opts {
		opt(job)
	}

	return job, nil
}

// jobModel 定义任务模型
type jobModel struct {
	name          string                // 任务名称
	spec          string                // cron 表达式
	schedule      parser.Schedule       // 解析后的cron表达式
	do            func()                // 执行函数
	doWithContext func(context.Context) // 支持上下文的执行函数
	async         bool                  // 是否异步执行
	tryCatch      bool                  // 是否进行异常捕获
	ctx           context.Context
	cancel        context.CancelFunc
	scheduler     *cronScheduler
	maxConcurrent int           // 最大并发数
	running       int32         // 当前运行的任务数
	timeout       time.Duration // 任务超时时间
	queue         chan struct{} // 并发控制队列
}

// SetTryCatch 设置是否启用异常捕获
func (j *jobModel) SetTryCatch(b bool) {
	j.tryCatch = b
}

// SetAsyncMode 设置是否异步执行
func (j *jobModel) SetAsyncMode(b bool) {
	j.async = b
}

// SetMaxConcurrent 设置最大并发数并初始化队列
func (j *jobModel) SetMaxConcurrent(n int) {
	j.maxConcurrent = n
	if n > 0 {
		j.queue = make(chan struct{}, n)
	}
}

// 验证任务模型的有效性
func (j *jobModel) validate() error {
	if j.do == nil {
		return ErrJobDOFuncNil
	}

	if j.schedule == nil {
		schedule, err := parser.NewParser(parser.Second | parser.Minute | parser.Hour | parser.Dom | parser.Month | parser.Dow).Parse(j.spec)
		if err != nil {
			return ErrInvalidSpec
		}
		j.schedule = schedule
	}

	return nil
}

// 启动任务循环
func (j *jobModel) runLoop(wg *sync.WaitGroup) {
	go j.run(wg)
}

// 执行任务
func (j *jobModel) run(wg *sync.WaitGroup) {
	defer wg.Done()

	// 获取下一次执行时间
	nextTime := j.schedule.Next(time.Now())
	if nextTime.IsZero() {
		defaultLogger.Error(fmt.Sprintf("Failed to get next execution time, job [%s] exited", j.name))
		return
	}

	// 创建定时器
	timer := time.NewTimer(time.Until(nextTime))
	defer timer.Stop()

	defaultLogger.Info(fmt.Sprintf("Job [%s] started, next execution time: %s", j.name, nextTime.Format(time.DateTime)))

	for atomic.LoadInt32(&j.running) > 0 {
		select {
		case <-timer.C:
			defaultLogger.Debug(fmt.Sprintf("Executing job [%s]", j.name))

			// 执行任务
			if j.async {
				go j.runWithTryCatch()
			} else {
				j.runWithTryCatch()
			}

			// 计算下一次执行时间
			nextTime = j.schedule.Next(time.Now())
			if nextTime.IsZero() {
				defaultLogger.Error(fmt.Sprintf("Failed to get next execution time, job [%s] exited", j.name))
				return
			}

			// 重置定时器
			timer.Reset(time.Until(nextTime))
			defaultLogger.Info(fmt.Sprintf("Job [%s] next execution time: %s", j.name, nextTime.Format(time.DateTime)))

		case <-j.ctx.Done():
			defaultLogger.Info(fmt.Sprintf("Job [%s] stopped", j.name))
			return
		}
	}
}

// 停止任务
func (j *jobModel) kill() {
	atomic.StoreInt32(&j.running, 0)
	if j.cancel != nil {
		j.cancel()
	}
}

// 执行带有异常捕获的任务
func (j *jobModel) runWithTryCatch() {
	// 检查并发数
	if j.async && j.maxConcurrent > 0 {
		currentRunning := atomic.LoadInt32(&j.running)
		if currentRunning >= int32(j.maxConcurrent) {
			defaultLogger.Warn(fmt.Sprintf(
				"Job [%s] skipped: current running tasks (%d) reached max concurrent limit (%d)",
				j.name, currentRunning, j.maxConcurrent,
			))
			return
		}

		// 使用缓冲队列控制并发，而不是简单丢弃
		select {
		case j.queue <- struct{}{}:
			// 成功进入队列
			defer func() { <-j.queue }()
		default:
			// 队列已满，任务等待
			defaultLogger.Info(fmt.Sprintf("Job [%s] waiting in queue", j.name))
			j.queue <- struct{}{} // 阻塞直到有空位
			defer func() { <-j.queue }()
		}
	}

	// 增加运行计数
	atomic.AddInt32(&j.running, 1)
	defer atomic.AddInt32(&j.running, -1)

	done := make(chan struct{})

	// 创建一个 context 用于取消操作
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// 执行任务
		startTime := time.Now()
		defaultLogger.Info(fmt.Sprintf("Starting execution of job [%s]", j.name))

		finished := make(chan struct{})
		go func() {
			defer func() {
				if r := recover(); r != nil && j.tryCatch {
					if j.scheduler != nil && j.scheduler.panicHandler != nil {
						j.scheduler.panicHandler.Handle(j.name, r)
					} else {
						defaultLogger.Error(fmt.Sprintf("Job [%s] panicked: %v", j.name, r))
					}
				}
				close(done)
			}()
			defer close(finished)

			// 优先使用支持上下文的函数
			if j.doWithContext != nil {
				// 创建一个可以传递超时的上下文
				execCtx := ctx
				if j.timeout > 0 {
					var cancel context.CancelFunc
					execCtx, cancel = context.WithTimeout(ctx, j.timeout)
					defer cancel()
				}
				j.doWithContext(execCtx)
			} else if j.do != nil {
				j.do()
			}
		}()

		// 等待任务完成或上下文取消
		select {
		case <-finished:
			elapsed := time.Since(startTime)
			defaultLogger.Info(fmt.Sprintf("Job [%s] execution completed, duration: %v", j.name, elapsed))
		case <-ctx.Done():
			defaultLogger.Error(fmt.Sprintf("Job [%s] cancelled", j.name))
		}
	}()

	// 处理超时
	if j.timeout > 0 {
		timer := time.NewTimer(j.timeout)
		defer timer.Stop()

		select {
		case <-done:
			// 任务正常完成，确保定时器资源被释放
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			// 超时，取消任务
			cancel()
			defaultLogger.Error(fmt.Sprintf("Job [%s] timed out (%v)", j.name, j.timeout))
			<-done // 等待任务真正结束
		}
	} else {
		<-done
	}
}

// 获取下次执行时间
func getNextDue(spec string) (time.Time, error) {
	sc, err := parser.NewParser(parser.Second | parser.Minute | parser.Hour | parser.Dom | parser.Month | parser.Dow).Parse(spec)
	if err != nil {
		return time.Time{}, err
	}
	return sc.Next(time.Now()), nil
}
