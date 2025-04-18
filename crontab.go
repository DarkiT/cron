package cron

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// 添加默认配置常量
const (
	DefaultMaxConcurrent = 1 // 默认最大并发数
)

// JobConfig 定义任务的配置参数
type JobConfig struct {
	Name          string        // 任务名称
	Schedule      string        // 定时规则
	Async         bool          // 是否异步执行
	TryCatch      bool          // 是否进行异常捕获
	Timeout       time.Duration // 任务超时时间
	MaxConcurrent int           // 最大并发数
}

// CronJob 定义了一个定时任务需要实现的接口
// 实现此接口的结构体可以被添加到定时任务管理器中
type CronJob interface {
	// Name 返回任务的唯一标识名称
	Name() string

	// Rule 返回任务的 cron 表达式
	Rule() string

	// Execute 执行任务
	Execute()
}

// CronJobWithContext 定义了支持上下文的任务接口
// 实现此接口的任务可以感知取消信号和超时
type CronJobWithContext interface {
	// Name 返回任务的唯一标识名称
	Name() string

	// Rule 返回任务的 cron 表达式
	Rule() string

	// ExecuteWithContext 接收上下文执行任务
	ExecuteWithContext(ctx context.Context)
}

// 存储任务信息的内部结构
type taskInfo struct {
	name string
	spec string
	fn   func()
}

// Crontab 定义了定时任务管理器的主要结构
type Crontab struct {
	scheduler *cronScheduler
	isRunning bool
	mu        sync.RWMutex
	tasks     map[string]taskInfo // 用于替代全局的fns变量
}

// New 创建一个新的定时任务管理器
//
// 可选参数 opts 用于配置管理器的行为，例如设置 panic 处理器：
//
//	scheduler := New(WithPanicHandler(&MyPanicHandler{}))
func New(opts ...Option) *Crontab {
	c := &Crontab{
		scheduler: NewCronScheduler(),
		tasks:     make(map[string]taskInfo),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Start 启动定时任务管理器
func (c *Crontab) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning {
		c.isRunning = true
		c.scheduler.Start()
	}
}

// Reload 重新加载所有任务
func (c *Crontab) Reload() {
	c.mu.Lock()
	tasksCopy := make(map[string]taskInfo)
	for k, v := range c.tasks {
		tasksCopy[k] = v
	}
	c.mu.Unlock()

	c.Stop()

	for _, v := range tasksCopy {
		c.AddFunc(v.name, v.spec, v.fn)
	}
	c.Start()
}

// Stop 停止定时任务管理器
func (c *Crontab) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.isRunning = false
	c.scheduler.Stop()
}

// AddJob 添加一个新的定时任务
//
// config 参数用于配置任务的属性，包括：
//   - Name: 任务的唯一标识名称
//   - Schedule: cron 表达式
//   - Async: 是否异步执行
//   - TryCatch: 是否捕获 panic
//
// fn 参数是任务的执行函数
//
// 示例:
//
//	err := scheduler.AddJob(crontab.JobConfig{
//	    Name:     "my-job",
//	    Schedule: "*/5 * * * * *",
//	    Async:    true,
//	}, func() {
//	    // 任务逻辑
//	})
func (c *Crontab) AddJob(config JobConfig, fn func()) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 存储任务信息
	c.tasks[config.Name] = taskInfo{
		name: config.Name,
		spec: config.Schedule,
		fn:   fn,
	}

	// 设置默认值
	if config.Async && config.MaxConcurrent <= 0 {
		config.MaxConcurrent = DefaultMaxConcurrent
	}

	// 如果设置了 panic 处理器，自动启用 TryCatch
	if c.scheduler.panicHandler != nil {
		config.TryCatch = true
	}

	// 创建任务模型
	job, err := NewJobModel(config.Schedule, fn,
		WithAsync(config.Async),
		WithTryCatch(config.TryCatch),
		WithTimeout(config.Timeout),
		WithMaxConcurrent(config.MaxConcurrent),
	)
	if err != nil {
		return fmt.Errorf("invalid job config: %w", err)
	}

	// 根据运行状态选择注册方式
	if c.isRunning {
		return c.scheduler.DynamicRegister(config.Name, job)
	}
	return c.scheduler.Register(config.Name, job)
}

// UpdateJob 更新已存在的定时任务
// 参数：
//   - config: 新的任务配置
//   - fn: 新的执行函数
//
// 返回：
//   - error: 如果更新失败则返回错误
func (c *Crontab) UpdateJob(config JobConfig, fn func()) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 更新任务信息
	c.tasks[config.Name] = taskInfo{
		name: config.Name,
		spec: config.Schedule,
		fn:   fn,
	}

	// 先停止旧任务
	c.scheduler.StopService(config.Name)

	// 创建新任务模型（暂不设置执行函数）
	job, err := NewJobModel(config.Schedule, nil,
		WithAsync(config.Async),
		WithTryCatch(config.TryCatch),
	)
	if err != nil {
		return fmt.Errorf("invalid job config: %w", err)
	}

	// 设置实际的执行函数，传入任务ID
	job.do = func() {
		fn()
	}

	// 注册新任务
	if c.isRunning {
		return c.scheduler.DynamicRegister(config.Name, job)
	}
	return c.scheduler.Register(config.Name, job)
}

// AddFunc 使用简单方式添加定时任务
// 参数：
//   - name: 任务名称
//   - spec: cron 表达式
//   - cmd: 要执行的函数
//
// 返回：
//   - error: 如果添加失败则返回错误
func (c *Crontab) AddFunc(name, spec string, fn func()) error {
	if fn == nil {
		return fmt.Errorf("task function not found")
	}

	// 存储任务信息
	c.mu.Lock()
	c.tasks[name] = taskInfo{
		name: name,
		spec: spec,
		fn:   fn,
	}
	c.mu.Unlock()

	job, err := NewJobModel(spec, fn)
	if err != nil {
		return fmt.Errorf("create job model failed: %w", err)
	}

	return c.scheduler.Register(name, job)
}

// AddJobInterface 添加实现了 Interface 接口的任务
// 参数：
//   - cmd: 实现了 Interface 接口的任务列表
//
// 返回：
//   - error: 如果添加失败则返回错误
func (c *Crontab) AddJobInterface(job CronJob) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}

	// 创建一个新的任务配置
	config := JobConfig{
		Name:     job.Name(),
		Schedule: job.Rule(),
		TryCatch: true,
	}

	// 检查是否实现了支持上下文的接口
	if jobWithCtx, ok := job.(CronJobWithContext); ok {
		// 使用支持上下文的接口
		ctxJob := func(ctx context.Context) {
			defaultLogger.Info(fmt.Sprintf("Starting execution of job [%s]", job.Name()))
			jobWithCtx.ExecuteWithContext(ctx)
			defaultLogger.Info(fmt.Sprintf("Completed execution of job [%s]", job.Name()))
		}

		// 创建任务模型
		jobModel, err := NewJobModel(config.Schedule, nil)
		if err != nil {
			return err
		}

		// 应用任务选项
		WithContextFunc(ctxJob)(jobModel)

		// 存储任务信息
		c.mu.Lock()
		c.tasks[config.Name] = taskInfo{
			name: config.Name,
			spec: config.Schedule,
			fn:   func() { ctxJob(context.Background()) },
		}
		c.mu.Unlock()

		// 注册任务
		if c.isRunning {
			return c.scheduler.DynamicRegister(config.Name, jobModel)
		}
		return c.scheduler.Register(config.Name, jobModel)
	}

	// 使用闭包捕获 job 变量
	jobFunc := func() {
		defaultLogger.Info(fmt.Sprintf("Starting execution of job [%s]", job.Name()))
		job.Execute()
		defaultLogger.Info(fmt.Sprintf("Completed execution of job [%s]", job.Name()))
	}

	return c.AddJob(config, jobFunc)
}

// NextRuntime 获取指定任务的下一次执行时间
// 参数：
//   - name: 任务名称
//
// 返回：
//   - time.Time: 下一次执行时间
//   - error: 如果获取失败则返回错误
func (c *Crontab) NextRuntime(name string) (time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 获取任务
	job, err := c.scheduler.GetServiceCron(name)
	if err != nil {
		return time.Time{}, fmt.Errorf("get job failed: %w", err)
	}

	// 获取下次执行时间
	nextTime, err := getNextDue(job.spec)
	if err != nil {
		return time.Time{}, fmt.Errorf("calculate next runtime failed: %w", err)
	}

	return nextTime, nil
}

// ListJobs 返回所有注册的任务名称
func (c *Crontab) ListJobs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var jobs []string
	for name := range c.scheduler.tasks {
		jobs = append(jobs, name)
	}
	return jobs
}

// GetJobStatus 返回任务的当前状态
// 参数：
//   - name: 要获取的任务名称
func (c *Crontab) GetJobStatus(name string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	job, exists := c.scheduler.tasks[name]
	if !exists {
		return false, fmt.Errorf("job %s not found", name)
	}

	return atomic.LoadInt32(&job.running) > 0, nil
}

// StopService 停止指定名称的任务
// 参数：
//   - name: 要停止的任务名称列表
func (c *Crontab) StopService(name ...string) {
	if len(name) == 0 {
		return
	}
	for _, fn := range name {
		c.scheduler.StopService(fn)
	}
}

// StopServicePrefix 停止所有指定前缀的任务
// 参数：
//   - namePrefix: 任务名称前缀
func (c *Crontab) StopServicePrefix(namePrefix string) {
	if len(namePrefix) == 0 {
		return
	}
	c.scheduler.StopServicePrefix(namePrefix)
}

// Register 直接注册一个任务模型
// 参数：
//   - name: 任务名称
//   - job: 任务模型
//
// 返回：
//   - error: 如果注册失败则返回错误
func (c *Crontab) Register(name string, job *jobModel) error {
	c.mu.Lock()
	c.tasks[name] = taskInfo{
		name: name,
		spec: job.spec,
		fn:   job.do,
	}
	c.mu.Unlock()

	return c.scheduler.Register(name, job)
}
