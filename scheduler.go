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
)

// 从 robfig/cron 复制 crontab 解析器到 crontab.cron_parser.go

var (
	ErrNotFoundJob     = errors.New("job not found")
	ErrAlreadyRegister = errors.New("job already exists")
	ErrJobDOFuncNil    = errors.New("callback function is nil")
)

// 设置默认的日志记录器
var defaultLogger = slog.Default()

// SetLogger 允许用户设置自定义日志记录器
func SetLogger(logger *slog.Logger) {
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

// Start - 启动所有任务，仅执行一次
func (c *cronScheduler) Start() {
	c.once.Do(func() {
		for _, job := range c.tasks {
			c.wg.Add(1)
			job.runLoop(c.wg)
		}
	})
}

// Wait - 等待所有任务退出
func (c *cronScheduler) Wait() {
	c.wg.Wait()
}

// WaitStop - 等待调度器停止
func (c *cronScheduler) WaitStop() {
	<-c.ctx.Done()
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

// SetPanicHandler - 设置 panic 处理器
func (c *cronScheduler) SetPanicHandler(handler PanicHandler) {
	c.panicHandler = handler
}

// NewJobModel 创建任务模型
func NewJobModel(spec string, fn func(), opts ...JobOption) (*jobModel, error) {
	job := &jobModel{
		spec:    spec,
		do:      fn,
		running: 1,
	}

	// 应用选项
	for _, opt := range opts {
		opt(job)
	}

	return job, nil
}

// jobModel 定义任务模型
type jobModel struct {
	name          string // 任务名称
	spec          string // cron 表达式
	do            func() // 执行函数
	async         bool   // 是否异步执行
	tryCatch      bool   // 是否进行异常捕获
	ctx           context.Context
	cancel        context.CancelFunc
	scheduler     *cronScheduler
	maxConcurrent int           // 最大并发数
	running       int32         // 当前运行的任务数
	timeout       time.Duration // 任务超时时间
}

// SetTryCatch 设置是否启用异常捕获
func (j *jobModel) SetTryCatch(b bool) {
	j.tryCatch = b
}

// SetAsyncMode 设置是否异步执行
func (j *jobModel) SetAsyncMode(b bool) {
	j.async = b
}

// 验证任务模型的有效性
func (j *jobModel) validate() error {
	if j.do == nil {
		return ErrJobDOFuncNil
	}

	if _, err := getNextDue(j.spec); err != nil {
		return err
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
	nextTime, err := getNextDue(j.spec)
	if err != nil {
		defaultLogger.Error(fmt.Sprintf("Failed to get next due time: %v", err))
		return
	}

	// 创建定时器
	timer := time.NewTimer(time.Until(nextTime))
	defer timer.Stop()

	defaultLogger.Info(fmt.Sprintf("Job [%s] started, next execution time: %s", j.name, nextTime.Format(time.DateTime)))

	for atomic.LoadInt32(&j.running) > 0 {
		select {
		case <-timer.C:
			defaultLogger.Debug(fmt.Sprintf("Job [%s] executing", j.name))

			// 执行任务
			if j.async {
				go j.runWithTryCatch()
			} else {
				j.runWithTryCatch()
			}

			// 计算下一次执行时间
			nextTime, err = getNextDue(j.spec)
			if err != nil {
				defaultLogger.Error(fmt.Sprintf("Failed to get next due time: %v", err))
				continue
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
		defaultLogger.Info(fmt.Sprintf("Executing job [%s]", j.name))

		finished := make(chan struct{})
		go func() {
			defer func() {
				if r := recover(); r != nil && j.tryCatch {
					if j.scheduler != nil && j.scheduler.panicHandler != nil {
						j.scheduler.panicHandler.Handle(j.name, r)
					}
				}
				close(done)
			}()
			defer close(finished)
			j.do()
		}()

		// 等待任务完成或上下文取消
		select {
		case <-finished:
			defaultLogger.Info(fmt.Sprintf("Job [%s] execution completed", j.name))
		case <-ctx.Done():
			defaultLogger.Error(fmt.Sprintf("Job [%s] cancelled", j.name))
		}
	}()

	// 处理超时
	if j.timeout > 0 {
		select {
		case <-done:
			// 任务正常完成
		case <-time.After(j.timeout):
			// 超时，取消任务
			cancel()
			defaultLogger.Error(fmt.Sprintf("Job [%s] timeout", j.name))
			<-done // 等待任务真正结束
		}
	} else {
		<-done
	}
}

// 获取下次执行时间
func getNextDue(spec string) (time.Time, error) {
	// 标准化 cron 表达式
	normalizedSpec := normalizeCronExpr(spec)

	sc, err := Parse(normalizedSpec)
	if err != nil {
		return time.Now(), err
	}

	due := sc.Next(time.Now())
	return due, err
}

// 处理 crontab 表达式，自动处理 5 段语法
func normalizeCronExpr(spec string) string {
	fields := strings.Fields(spec)
	if len(fields) == 5 {
		// 5段语法，在前面补充 "0" 作为秒位
		return "0 " + spec
	}
	if len(fields) == 6 {
		// 6段语法，原样返回
		return spec
	}
	return spec // 错误的表达式会在后续解析时报错
}
