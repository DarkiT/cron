package cron

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// 常用的 cron 表达式
const (
	EverySecond = "* * * * * *"
	EveryMinute = "0 * * * * *"
	EveryHour   = "0 0 * * * *"
	EveryDay    = "0 0 0 * * *"
	EveryWeek   = "0 0 0 * * 0"
	EveryMonth  = "0 0 0 1 * *"

	// 常用间隔
	Every5Seconds  = "*/5 * * * * *"
	Every10Seconds = "*/10 * * * * *"
	Every15Seconds = "*/15 * * * * *"
	Every30Seconds = "*/30 * * * * *"
	Every5Minutes  = "0 */5 * * * *"
	Every10Minutes = "0 */10 * * * *"
	Every15Minutes = "0 */15 * * * *"
	Every30Minutes = "0 */30 * * * *"
)

// Job 定义任务接口（简化版）
type Job interface {
	Run(ctx context.Context) error // 执行任务，接收上下文并返回错误
	Name() string                  // 返回任务名称，用于标识任务
}

// JobOptions 任务配置选项
type JobOptions struct {
	Timeout       time.Duration // 任务超时时间
	MaxRetries    int           // 最大重试次数
	Async         bool          // 是否异步执行
	MaxConcurrent int           // 最大并发数
}

// Logger 定义日志接口
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Option 定义创建选项
type Option func(*Cron)

// WithLogger 设置自定义日志接口
func WithLogger(logger Logger) Option {
	return func(c *Cron) {
		c.logger = logger
	}
}

// WithContext 设置调度器的根上下文，用于生命周期管理
// 当上下文被取消时，调度器将停止调度新任务，并向所有正在执行的任务发送取消信号
func WithContext(ctx context.Context) Option {
	return func(c *Cron) {
		if ctx == nil {
			if c.logger != nil {
				c.logger.Warnf("WithContext received nil context, using Background")
			}
			ctx = context.Background()
		}
		c.rootContext = ctx
	}
}

// Cron 是一个极简的定时任务调度器
type Cron struct {
	scheduler    *scheduler
	mu           sync.RWMutex
	running      bool
	logger       Logger
	monitor      *Monitor
	startTime    time.Time
	panicHandler PanicHandler
	rootContext  context.Context // 根上下文，用于生命周期管理
}

// New 创建一个新的定时任务调度器
func New(opts ...Option) *Cron {
	defaultLog := NewDefaultLogger()
	c := &Cron{
		logger:      defaultLog,
		startTime:   time.Now(),
		rootContext: context.Background(), // 默认使用 Background
	}

	// 应用选项
	for _, opt := range opts {
		opt(c)
	}

	if c.panicHandler == nil {
		c.panicHandler = NewDefaultPanicHandler(c.logger)
	}

	// 使用配置的 rootContext 创建调度器
	c.scheduler = newSchedulerWithContext(c.rootContext)

	// 默认启用监控
	c.enableMonitoring()

	// 设置调度器的依赖项
	c.scheduler.logger = c.logger
	c.scheduler.monitor = c.monitor
	c.scheduler.panicHandler = c.panicHandler

	return c
}

// Schedule 添加一个任务到调度器 - 最简化的核心API
func (c *Cron) Schedule(id, schedule string, handler func(ctx context.Context)) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	task := &Task{
		ID:       id,
		Schedule: schedule,
		Handler:  handler,
		created:  time.Now(),
	}

	// 添加到监控
	if c.monitor != nil {
		c.monitor.addTask(id, schedule, time.Now())
	}

	return c.scheduler.addTask(task)
}

// ScheduleJob 添加一个实现了Job接口的任务
func (c *Cron) ScheduleJob(id, schedule string, job Job, opts ...JobOptions) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	task := &Task{
		ID:       id,
		Schedule: schedule,
		Job:      job,
		created:  time.Now(),
	}

	// 应用选项
	if len(opts) > 0 {
		task.Options = opts[0]
	}

	// 添加到监控
	if c.monitor != nil {
		c.monitor.addTask(id, schedule, time.Now())
	}

	return c.scheduler.addTask(task)
}

// ScheduleJobByName 使用Job的Name()方法自动获取任务名称进行调度
// 更简洁优雅的API：c.ScheduleJobByName("0 2 * * *", job.NewBackup())
func (c *Cron) ScheduleJobByName(schedule string, job Job, opts ...JobOptions) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	// 使用Job的Name()方法获取任务名称
	return c.ScheduleJob(job.Name(), schedule, job, opts...)
}

// Remove 移除一个定时任务
func (c *Cron) Remove(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 从监控中移除
	if c.monitor != nil {
		c.monitor.removeTask(id)
	}

	return c.scheduler.removeTask(id)
}

// Start 启动调度器
func (c *Cron) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("scheduler is already running")
	}

	c.running = true
	c.logger.Infof("Starting cron scheduler")

	// 启动调度器
	err := c.scheduler.start()
	if err != nil {
		c.running = false
		return err
	}

	// 启动上下文监听器
	go c.contextWatcher()

	return nil
}

// contextWatcher 监听根上下文的取消信号
func (c *Cron) contextWatcher() {
	if c.rootContext == nil {
		return
	}

	// 检查 Done channel 是否为 nil
	// context.Background()和 context.TODO()的 Done() 返回 nil
	done := c.rootContext.Done()
	if done == nil {
		return
	}

	<-done

	// 上下文被取消，自动停止调度器
	if c.logger != nil {
		c.logger.Infof("Root context cancelled, stopping cron scheduler")
	}
	c.Stop()
}

// Stop 停止调度器
func (c *Cron) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.running = false
	c.logger.Infof("Stopping cron scheduler")
	c.scheduler.stop()
}

// List 列出所有任务名称
func (c *Cron) List() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.scheduler.listTasks()
}

// NextRun 获取任务的下次执行时间
func (c *Cron) NextRun(id string) (time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.scheduler.nextRun(id)
}

// IsRunning 检查调度器是否正在运行
func (c *Cron) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.running
}

// Task 定义一个任务（简化版）
type Task struct {
	ID       string                    // 任务ID
	Schedule string                    // cron表达式
	Handler  func(ctx context.Context) // 任务处理函数
	Job      Job                       // 任务接口实现
	Options  JobOptions                // 任务配置
	created  time.Time                 // 创建时间
}

// enableMonitoring 启用监控
func (c *Cron) enableMonitoring() {
	if c.monitor == nil {
		c.monitor = newMonitor()
		c.startTime = time.Now()
	}
}

// GetStats 获取指定任务的统计信息
func (c *Cron) GetStats(id string) (*Stats, bool) {
	if c.monitor == nil {
		return nil, false
	}
	return c.monitor.GetStats(id)
}

// GetAllStats 获取所有任务的统计信息
func (c *Cron) GetAllStats() map[string]*Stats {
	if c.monitor == nil {
		return make(map[string]*Stats)
	}
	return c.monitor.GetAllStats()
}
