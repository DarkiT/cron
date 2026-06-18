package cron

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/darkit/cron/history"
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

// MisfirePolicy 定义 Misfire 处理策略
type MisfirePolicy string

const (
	MisfireSkip    MisfirePolicy = "skip"    // 跳过落后触发
	MisfireRunOnce MisfirePolicy = "once"    // 仅补跑一次
	MisfireCatchUp MisfirePolicy = "catchup" // 尝试追赶，最多补若干次
)

// JobOptions 任务配置选项
type JobOptions struct {
	Timeout       time.Duration     // 任务超时时间
	MaxRetries    int               // 最大重试次数，-1 表示无限重试，0 表示不重试
	RetryInterval time.Duration     // 重试间隔时间，0 表示立即重试
	Async         bool              // 是否异步执行
	MaxConcurrent int               // 最大并发数
	MisfirePolicy MisfirePolicy     // Misfire 策略
	MaxCatchUp    int               // MisfireCatchUp 策略的最大补跑次数，0 表示使用默认值 5
	FailThreshold int               // 连续失败阈值，达到后触发暂停
	FailWindow    time.Duration     // 统计失败的时间窗口，0 表示不限窗口
	PauseDuration time.Duration     // 自动暂停时长
	StartAt       time.Time         // 首次执行时间，零值表示沿用默认首次调度行为
	MaxRuns       int               // 最大计划执行次数，0 表示不限次数
	Labels        map[string]string // 任务标签元数据
}

// EventHook 任务事件回调
type EventHook func(Event)

// Event 任务事件数据
type Event struct {
	TaskID   string
	Start    time.Time
	End      time.Time
	Success  bool
	Error    string
	Retries  int
	Duration time.Duration
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
		if logger == nil {
			return
		}
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

// WithHistoryRecorder 启用历史记录功能。
// recorder 的生命周期由调用方管理，Cron 在 Stop 时不会主动关闭它。
func WithHistoryRecorder(recorder history.Recorder) Option {
	return func(c *Cron) {
		if recorder == nil {
			return
		}
		c.recorder = recorder
	}
}

// WithEventHook 设置任务事件回调
func WithEventHook(hook EventHook) Option {
	return func(c *Cron) {
		c.eventHook = hook
	}
}

// Cron 是一个极简的定时任务调度器
type Cron struct {
	scheduler    *scheduler
	mu           sync.RWMutex
	running      bool
	closed       bool
	logger       Logger
	monitor      *Monitor
	startTime    time.Time
	panicHandler PanicHandler
	rootContext  context.Context  // 根上下文，用于生命周期管理
	recorder     history.Recorder // 历史记录器（可选）
	eventHook    EventHook
	watcherStop  chan struct{}
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
	c.watcherStop = make(chan struct{})

	// 默认启用监控
	c.enableMonitoring()

	// 设置调度器的依赖项
	c.scheduler.logger = c.logger
	c.scheduler.monitor = c.monitor
	c.scheduler.panicHandler = c.panicHandler
	c.scheduler.recorder = c.recorder
	c.scheduler.eventHook = c.eventHook

	return c
}

func normalizeTaskID(id string) (string, error) {
	normalizedID := strings.TrimSpace(id)
	if normalizedID == "" {
		return "", fmt.Errorf("task id cannot be empty")
	}
	if normalizedID == "." || normalizedID == ".." {
		return "", fmt.Errorf("task id %q is invalid", normalizedID)
	}
	if strings.ContainsAny(normalizedID, `/\\`) {
		return "", fmt.Errorf("task id %q cannot contain path separators", normalizedID)
	}
	return normalizedID, nil
}

func normalizeScheduleSpec(schedule string) (string, error) {
	normalizedSchedule := strings.TrimSpace(schedule)
	if normalizedSchedule == "" {
		return "", fmt.Errorf("schedule cannot be empty")
	}
	return normalizedSchedule, nil
}

func normalizeTaskInputs(id, schedule string) (string, string, error) {
	normalizedID, err := normalizeTaskID(id)
	if err != nil {
		return "", "", err
	}
	normalizedSchedule, err := normalizeScheduleSpec(schedule)
	if err != nil {
		return "", "", err
	}

	return normalizedID, normalizedSchedule, nil
}

func normalizeJobOptions(opts JobOptions) (JobOptions, error) {
	if opts.Timeout < 0 {
		return JobOptions{}, fmt.Errorf("timeout cannot be negative")
	}
	if opts.MaxRetries < -1 {
		return JobOptions{}, fmt.Errorf("max retries must be -1 or greater")
	}
	if opts.RetryInterval < 0 {
		return JobOptions{}, fmt.Errorf("retry interval cannot be negative")
	}
	if opts.MaxConcurrent < 0 {
		return JobOptions{}, fmt.Errorf("max concurrent cannot be negative")
	}
	if opts.MaxCatchUp < 0 {
		return JobOptions{}, fmt.Errorf("max catch up cannot be negative")
	}
	if opts.FailThreshold < 0 {
		return JobOptions{}, fmt.Errorf("fail threshold cannot be negative")
	}
	if opts.FailWindow < 0 {
		return JobOptions{}, fmt.Errorf("fail window cannot be negative")
	}
	if opts.PauseDuration < 0 {
		return JobOptions{}, fmt.Errorf("pause duration cannot be negative")
	}
	if opts.MaxRuns < 0 {
		return JobOptions{}, fmt.Errorf("max runs cannot be negative")
	}

	switch opts.MisfirePolicy {
	case "":
		opts.MisfirePolicy = MisfireSkip
	case MisfireSkip, MisfireRunOnce, MisfireCatchUp:
	default:
		return JobOptions{}, fmt.Errorf("invalid misfire policy %q", opts.MisfirePolicy)
	}

	return cloneJobOptions(opts), nil
}

func mergeJobOptions(opts []JobOptions, mutate func(*JobOptions) error) (JobOptions, error) {
	if len(opts) > 1 {
		return JobOptions{}, fmt.Errorf("at most one JobOptions value may be provided")
	}

	merged := JobOptions{}
	if len(opts) > 0 {
		normalized, err := normalizeJobOptions(opts[0])
		if err != nil {
			return JobOptions{}, err
		}
		merged = normalized
	}

	if mutate != nil {
		if err := mutate(&merged); err != nil {
			return JobOptions{}, err
		}
	}

	return normalizeJobOptions(merged)
}

func ensureReservedOption(name string, current, expected int) error {
	if current != 0 && current != expected {
		return fmt.Errorf("job option %s conflicts with helper API", name)
	}
	return nil
}

// Schedule 添加一个任务到调度器 - 最简化的核心API
func (c *Cron) Schedule(id, schedule string, handler func(ctx context.Context), opts ...JobOptions) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	normalizedID, normalizedSchedule, err := normalizeTaskInputs(id, schedule)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("scheduler is closed")
	}

	taskOptions := JobOptions{MisfirePolicy: MisfireSkip}
	if len(opts) > 1 {
		return fmt.Errorf("at most one JobOptions value may be provided")
	}
	if len(opts) > 0 {
		taskOptions, err = normalizeJobOptions(opts[0])
		if err != nil {
			return err
		}
	}

	createdAt := time.Now()
	task := &Task{
		ID:       normalizedID,
		Schedule: normalizedSchedule,
		Handler:  handler,
		Options:  taskOptions,
		Labels:   cloneLabels(taskOptions.Labels),
		created:  createdAt,
	}

	if err := c.scheduler.addTask(task); err != nil {
		return err
	}

	// 任务添加成功后再写监控，避免脏状态
	if c.monitor != nil {
		c.monitor.addTask(normalizedID, normalizedSchedule, createdAt, task.Labels, string(task.Options.MisfirePolicy))
	}

	return nil
}

// ScheduleJob 添加一个实现了Job接口的任务
func (c *Cron) ScheduleJob(id, schedule string, job Job, opts ...JobOptions) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	normalizedID, normalizedSchedule, err := normalizeTaskInputs(id, schedule)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("scheduler is closed")
	}

	taskOptions := JobOptions{MisfirePolicy: MisfireSkip}
	if len(opts) > 1 {
		return fmt.Errorf("at most one JobOptions value may be provided")
	}
	if len(opts) > 0 {
		taskOptions, err = normalizeJobOptions(opts[0])
		if err != nil {
			return err
		}
	}

	createdAt := time.Now()
	task := &Task{
		ID:       normalizedID,
		Schedule: normalizedSchedule,
		Job:      job,
		Options:  taskOptions,
		Labels:   cloneLabels(taskOptions.Labels),
		created:  createdAt,
	}

	if err := c.scheduler.addTask(task); err != nil {
		return err
	}

	// 任务添加成功后再写监控，避免脏状态
	if c.monitor != nil {
		c.monitor.addTask(normalizedID, normalizedSchedule, createdAt, task.Labels, string(task.Options.MisfirePolicy))
	}

	return nil
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

// ScheduleOnceAt 在指定时间执行一次任务，并在完成后自动移除。
func (c *Cron) ScheduleOnceAt(id string, runAt time.Time, handler func(ctx context.Context), opts ...JobOptions) error {
	taskOptions, err := mergeJobOptions(opts, func(o *JobOptions) error {
		if runAt.IsZero() {
			return fmt.Errorf("runAt cannot be zero")
		}
		if !o.StartAt.IsZero() && !o.StartAt.Equal(runAt) {
			return fmt.Errorf("job option StartAt conflicts with helper API")
		}
		if err := ensureReservedOption("MaxRuns", o.MaxRuns, 1); err != nil {
			return err
		}
		o.StartAt = runAt
		o.MaxRuns = 1
		return nil
	})
	if err != nil {
		return err
	}

	return c.Schedule(id, "@every 1s", handler, taskOptions)
}

// ScheduleJobOnceAt 在指定时间执行一次 Job，并在完成后自动移除。
func (c *Cron) ScheduleJobOnceAt(id string, runAt time.Time, job Job, opts ...JobOptions) error {
	taskOptions, err := mergeJobOptions(opts, func(o *JobOptions) error {
		if runAt.IsZero() {
			return fmt.Errorf("runAt cannot be zero")
		}
		if !o.StartAt.IsZero() && !o.StartAt.Equal(runAt) {
			return fmt.Errorf("job option StartAt conflicts with helper API")
		}
		if err := ensureReservedOption("MaxRuns", o.MaxRuns, 1); err != nil {
			return err
		}
		o.StartAt = runAt
		o.MaxRuns = 1
		return nil
	})
	if err != nil {
		return err
	}

	return c.ScheduleJob(id, "@every 1s", job, taskOptions)
}

// ScheduleLimited 按计划执行指定次数，并在次数耗尽后自动移除。
func (c *Cron) ScheduleLimited(id, schedule string, maxRuns int, handler func(ctx context.Context), opts ...JobOptions) error {
	taskOptions, err := mergeJobOptions(opts, func(o *JobOptions) error {
		if maxRuns <= 0 {
			return fmt.Errorf("max runs must be greater than zero")
		}
		if err := ensureReservedOption("MaxRuns", o.MaxRuns, maxRuns); err != nil {
			return err
		}
		o.MaxRuns = maxRuns
		return nil
	})
	if err != nil {
		return err
	}

	return c.Schedule(id, schedule, handler, taskOptions)
}

// ScheduleLimitedFrom 在指定首次执行时间开始，按计划执行指定次数，并在次数耗尽后自动移除。
func (c *Cron) ScheduleLimitedFrom(id, schedule string, startAt time.Time, maxRuns int, handler func(ctx context.Context), opts ...JobOptions) error {
	taskOptions, err := mergeJobOptions(opts, func(o *JobOptions) error {
		if startAt.IsZero() {
			return fmt.Errorf("startAt cannot be zero")
		}
		if maxRuns <= 0 {
			return fmt.Errorf("max runs must be greater than zero")
		}
		if !o.StartAt.IsZero() && !o.StartAt.Equal(startAt) {
			return fmt.Errorf("job option StartAt conflicts with helper API")
		}
		if err := ensureReservedOption("MaxRuns", o.MaxRuns, maxRuns); err != nil {
			return err
		}
		o.StartAt = startAt
		o.MaxRuns = maxRuns
		return nil
	})
	if err != nil {
		return err
	}

	return c.Schedule(id, schedule, handler, taskOptions)
}

// ScheduleLimitedJob 按计划执行指定次数的 Job，并在次数耗尽后自动移除。
func (c *Cron) ScheduleLimitedJob(id, schedule string, maxRuns int, job Job, opts ...JobOptions) error {
	taskOptions, err := mergeJobOptions(opts, func(o *JobOptions) error {
		if maxRuns <= 0 {
			return fmt.Errorf("max runs must be greater than zero")
		}
		if err := ensureReservedOption("MaxRuns", o.MaxRuns, maxRuns); err != nil {
			return err
		}
		o.MaxRuns = maxRuns
		return nil
	})
	if err != nil {
		return err
	}

	return c.ScheduleJob(id, schedule, job, taskOptions)
}

// ScheduleLimitedJobFrom 在指定首次执行时间开始，按计划执行指定次数的 Job，并在次数耗尽后自动移除。
func (c *Cron) ScheduleLimitedJobFrom(id, schedule string, startAt time.Time, maxRuns int, job Job, opts ...JobOptions) error {
	taskOptions, err := mergeJobOptions(opts, func(o *JobOptions) error {
		if startAt.IsZero() {
			return fmt.Errorf("startAt cannot be zero")
		}
		if maxRuns <= 0 {
			return fmt.Errorf("max runs must be greater than zero")
		}
		if !o.StartAt.IsZero() && !o.StartAt.Equal(startAt) {
			return fmt.Errorf("job option StartAt conflicts with helper API")
		}
		if err := ensureReservedOption("MaxRuns", o.MaxRuns, maxRuns); err != nil {
			return err
		}
		o.StartAt = startAt
		o.MaxRuns = maxRuns
		return nil
	})
	if err != nil {
		return err
	}

	return c.ScheduleJob(id, schedule, job, taskOptions)
}

// Remove 移除一个定时任务
func (c *Cron) Remove(id string) error {
	normalizedID, err := normalizeTaskID(id)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("scheduler is closed")
	}

	if err := c.scheduler.removeTask(normalizedID); err != nil {
		return err
	}

	// 从监控中移除
	if c.monitor != nil {
		c.monitor.removeTask(normalizedID)
	}

	return nil
}

// Start 启动调度器
func (c *Cron) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("scheduler is closed")
	}

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
	c.watcherStop = make(chan struct{})
	go c.contextWatcher(c.watcherStop)

	return nil
}

// contextWatcher 监听根上下文的取消信号
func (c *Cron) contextWatcher(stopCh <-chan struct{}) {
	if c.rootContext == nil {
		return
	}

	// 检查 Done channel 是否为 nil
	// context.Background()和 context.TODO()的 Done() 返回 nil
	done := c.rootContext.Done()
	if done == nil {
		return
	}

	select {
	case <-done:
		// 上下文被取消，自动停止调度器
		if c.logger != nil {
			c.logger.Infof("Root context cancelled, stopping cron scheduler")
		}
		c.Stop()
	case <-stopCh:
		return
	}
}

// Stop 停止调度器
func (c *Cron) Stop() {
	c.stopInternal(5*time.Second, "Stopping cron scheduler")
}

// StopGracefully 带超时的优雅停止，等待正在执行的任务完成
// timeout <= 0 时表示无限等待
func (c *Cron) StopGracefully(timeout time.Duration) {
	c.stopInternal(timeout, "Stopping cron scheduler gracefully")
}

// stopInternal 统一的停止流程，避免重复代码
func (c *Cron) stopInternal(timeout time.Duration, logMsg string) {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}

	c.running = false
	stopCh := c.watcherStop
	c.watcherStop = nil
	if c.logger != nil {
		c.logger.Infof(logMsg)
	}
	sched := c.scheduler
	c.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}

	sched.stopWithTimeout(timeout)
}

// Close 关闭调度器并释放托管资源。调用后实例不可再复用。
func (c *Cron) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	c.stopInternal(5*time.Second, "Closing cron scheduler")

	c.mu.Lock()
	recorder := c.recorder
	c.recorder = nil
	if c.scheduler != nil {
		c.scheduler.recorder = nil
	}
	c.mu.Unlock()

	if recorder != nil {
		if err := recorder.Close(); err != nil {
			if c.logger != nil {
				c.logger.Warnf("Failed to close history recorder: %v", err)
			}
			return err
		}
	}
	return nil
}

// List 列出所有任务名称
func (c *Cron) List() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.scheduler.listTasks()
}

// NextRun 获取任务的下次执行时间
func (c *Cron) NextRun(id string) (time.Time, error) {
	normalizedID, err := normalizeTaskID(id)
	if err != nil {
		return time.Time{}, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.scheduler.nextRun(normalizedID)
}

// RunNow 立即触发指定任务一次
func (c *Cron) RunNow(id string) error {
	normalizedID, err := normalizeTaskID(id)
	if err != nil {
		return err
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("scheduler is closed")
	}
	sched := c.scheduler
	c.mu.RUnlock()

	if sched == nil {
		return fmt.Errorf("scheduler is not initialized")
	}
	return sched.runNow(normalizedID)
}

// Update 更新任务的调度表达式及可选配置
func (c *Cron) Update(id, schedule string, opts ...JobOptions) error {
	normalizedID, normalizedSchedule, err := normalizeTaskInputs(id, schedule)
	if err != nil {
		return err
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("scheduler is closed")
	}
	sched := c.scheduler
	c.mu.RUnlock()

	if sched == nil {
		return fmt.Errorf("scheduler is not initialized")
	}

	var option *JobOptions
	if len(opts) > 1 {
		return fmt.Errorf("at most one JobOptions value may be provided")
	}
	if len(opts) > 0 {
		normalizedOpts, err := normalizeJobOptions(opts[0])
		if err != nil {
			return err
		}
		option = &normalizedOpts
	}

	return sched.updateTask(normalizedID, normalizedSchedule, option)
}

// Pause 暂停任务调度
func (c *Cron) Pause(id string) error {
	normalizedID, err := normalizeTaskID(id)
	if err != nil {
		return err
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("scheduler is closed")
	}
	sched := c.scheduler
	c.mu.RUnlock()

	if sched == nil {
		return fmt.Errorf("scheduler is not initialized")
	}
	return sched.pauseTask(normalizedID)
}

// Resume 恢复已暂停的任务
func (c *Cron) Resume(id string) error {
	normalizedID, err := normalizeTaskID(id)
	if err != nil {
		return err
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("scheduler is closed")
	}
	sched := c.scheduler
	c.mu.RUnlock()

	if sched == nil {
		return fmt.Errorf("scheduler is not initialized")
	}
	return sched.resumeTask(normalizedID)
}

// IsRunning 检查调度器是否正在运行
func (c *Cron) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.running
}

// TaskInfo 任务详细信息
type TaskInfo struct {
	ID            string            // 任务ID
	Schedule      string            // cron表达式
	Options       JobOptions        // 任务配置
	Labels        map[string]string // 元数据标签
	NextRun       time.Time         // 下次执行时间
	RemainingRuns int               // 剩余计划执行次数，-1 表示无限制
	IsPaused      bool              // 是否暂停
	IsRunning     bool              // 是否正在运行
	CreatedAt     time.Time         // 创建时间
}

// GetTask 获取指定任务的详细信息
func (c *Cron) GetTask(id string) (*TaskInfo, bool) {
	normalizedID, err := normalizeTaskID(id)
	if err != nil {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.scheduler.getTaskInfo(normalizedID)
}

// GetAllTasks 获取所有任务的详细信息
func (c *Cron) GetAllTasks() []*TaskInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.scheduler.getAllTaskInfo()
}

// PauseAll 暂停所有任务
func (c *Cron) PauseAll() {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return
	}
	sched := c.scheduler
	c.mu.RUnlock()

	if sched == nil {
		return
	}
	sched.pauseAll()
}

// ResumeAll 恢复所有任务
func (c *Cron) ResumeAll() {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return
	}
	sched := c.scheduler
	c.mu.RUnlock()

	if sched == nil {
		return
	}
	sched.resumeAll()
}

// Task 定义一个任务（简化版）
type Task struct {
	ID       string                    // 任务ID
	Schedule string                    // cron表达式
	Handler  func(ctx context.Context) // 任务处理函数
	Job      Job                       // 任务接口实现
	Options  JobOptions                // 任务配置
	Labels   map[string]string         // 元数据标签
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

// QueryHistory 查询任务执行历史记录
func (c *Cron) QueryHistory(filter history.RecordFilter) ([]*history.ExecutionRecord, error) {
	if c.recorder == nil {
		return nil, fmt.Errorf("history recorder is not enabled")
	}
	return c.recorder.Query(filter)
}

// CountHistory 统计历史记录数量
func (c *Cron) CountHistory(filter history.RecordFilter) (int, error) {
	if c.recorder == nil {
		return 0, fmt.Errorf("history recorder is not enabled")
	}
	return c.recorder.Count(filter)
}

// CleanupHistory 清理指定时间之前的历史记录
func (c *Cron) CleanupHistory(before time.Time) (int, error) {
	if c.recorder == nil {
		return 0, fmt.Errorf("history recorder is not enabled")
	}
	return c.recorder.Cleanup(before)
}
