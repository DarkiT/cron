package cron

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/darkit/cron/internal/parser"
)

// scheduler 是核心调度器
type scheduler struct {
	tasks        map[string]*taskRunner
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	running      bool
	logger       Logger
	monitor      *Monitor
	panicHandler PanicHandler
	rootCtx      context.Context
}

// newScheduler 创建一个新的调度器
func newScheduler() *scheduler {
	return newSchedulerWithContext(context.Background())
}

// newSchedulerWithContext 使用指定的根上下文创建调度器
func newSchedulerWithContext(rootCtx context.Context) *scheduler {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(rootCtx)
	return &scheduler{
		tasks:   make(map[string]*taskRunner),
		ctx:     ctx,
		cancel:  cancel,
		rootCtx: rootCtx,
	}
}

// taskRunner 运行任务的实体
type taskRunner struct {
	task      *Task
	schedule  parser.Schedule
	nextRun   time.Time
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	semaphore chan struct{} // 并发控制
}

// addTask 添加一个任务
func (s *scheduler) addTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[task.ID]; exists {
		return fmt.Errorf("task %s already exists", task.ID)
	}

	// 解析cron表达式
	var schedule parser.Schedule
	var err error

	specFields := len(strings.Split(strings.TrimSpace(task.Schedule), " "))

	if specFields == 5 {
		schedule, err = parser.ParseStandard(task.Schedule)
	} else {
		p := parser.NewParser(parser.Second | parser.Minute | parser.Hour | parser.Dom | parser.Month | parser.Dow | parser.Descriptor)
		schedule, err = p.Parse(task.Schedule)
	}

	if err != nil {
		return fmt.Errorf("invalid cron spec %s: %w", task.Schedule, err)
	}

	// 创建任务运行器
	ctx, cancel := context.WithCancel(s.ctx)
	runner := &taskRunner{
		task:     task,
		schedule: schedule,
		nextRun:  schedule.Next(time.Now()),
		ctx:      ctx,
		cancel:   cancel,
	}

	// 为MaxConcurrent > 0的情况预先创建semaphore
	if task.Options.MaxConcurrent > 0 {
		runner.semaphore = make(chan struct{}, task.Options.MaxConcurrent)
	}

	s.tasks[task.ID] = runner

	// 如果调度器正在运行，立即启动任务
	if s.running {
		s.wg.Add(1)
		go s.runTask(runner)
	}

	return nil
}

// removeTask 移除一个任务
func (s *scheduler) removeTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	runner, exists := s.tasks[id]
	if !exists {
		return fmt.Errorf("task %s not found", id)
	}

	// 停止任务
	runner.cancel()
	delete(s.tasks, id)

	return nil
}

// start 启动调度器
func (s *scheduler) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.running = true

	// 启动所有任务
	for _, runner := range s.tasks {
		s.wg.Add(1)
		go s.runTask(runner)
	}

	return nil
}

// stop 停止调度器
func (s *scheduler) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	s.cancel()
	s.wg.Wait()

	// 重新构建上下文，允许后续重新启动
	s.ctx, s.cancel = context.WithCancel(s.rootCtx)
	for _, runner := range s.tasks {
		runnerCtx, cancel := context.WithCancel(s.ctx)
		runner.ctx = runnerCtx
		runner.cancel = cancel
		runner.mu.Lock()
		runner.running = false
		runner.nextRun = runner.schedule.Next(time.Now())
		runner.mu.Unlock()
	}
}

// listTasks 列出所有任务ID
func (s *scheduler) listTasks() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.tasks))
	for id := range s.tasks {
		ids = append(ids, id)
	}
	return ids
}

// nextRun 获取任务的下次执行时间
func (s *scheduler) nextRun(id string) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runner, exists := s.tasks[id]
	if !exists {
		return time.Time{}, fmt.Errorf("task %s not found", id)
	}

	runner.mu.RLock()
	defer runner.mu.RUnlock()

	return runner.nextRun, nil
}

// runTask 运行单个任务
func (s *scheduler) runTask(runner *taskRunner) {
	defer s.wg.Done()

	for {
		select {
		case <-runner.ctx.Done():
			return
		case <-time.After(time.Until(runner.nextRun)):
			s.executeTask(runner)

			// 计算下次运行时间
			runner.mu.Lock()
			runner.nextRun = runner.schedule.Next(time.Now())
			runner.mu.Unlock()
		}
	}
}

// executeTaskJob 执行任务的实际方法
func (s *scheduler) executeTaskJob(task *Task, ctx context.Context) {
	startTime := time.Now()
	success := false

	// 设置任务为运行状态
	if s.monitor != nil {
		s.monitor.setRunning(task.ID, true)
	}

	defer func() {
		// 记录执行统计
		duration := time.Since(startTime)
		if s.monitor != nil {
			s.monitor.recordExecution(task.ID, duration, success)
			s.monitor.setRunning(task.ID, false)
		}
	}()

	// 执行任务
	if task.Handler != nil {
		success = s.executeHandler(task, ctx)
	} else if task.Job != nil {
		success = s.executeJobInterface(task, ctx)
	}
}

// executeTask 执行任务
func (s *scheduler) executeTask(runner *taskRunner) {
	// 并发控制逻辑
	task := runner.task

	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			if s.panicHandler != nil {
				s.panicHandler.HandlePanic(task.ID, r, stack)
			} else if s.logger != nil {
				s.logger.Errorf("Task %s panicked: %v", task.ID, r)
			}
		}
	}()

	release := func() {}

	if task.Options.MaxConcurrent > 0 {
		// MaxConcurrent > 0: 严格限制最大并发数，超过则立即放弃任务
		if runner.semaphore == nil {
			// 初始化信号量
			runner.semaphore = make(chan struct{}, task.Options.MaxConcurrent)
		}

		select {
		case runner.semaphore <- struct{}{}:
			// 获得执行权限
			release = func() {
				<-runner.semaphore
			}
		default:
			// 超过并发限制，立即放弃任务
			if s.logger != nil {
				s.logger.Warnf("Task %s skipped due to concurrency limit (%d)", task.ID, task.Options.MaxConcurrent)
			}
			return
		}
	}
	// MaxConcurrent = 0: 允许无限并发，不做任何限制

	run := func() {
		defer release()

		execCtx := runner.ctx
		if task.Options.Timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(execCtx, task.Options.Timeout)
			defer cancel()
		}

		s.executeTaskJob(task, execCtx)
	}

	if task.Options.Async {
		go run()
	} else {
		run()
	}
}

// executeHandler 执行处理函数
func (s *scheduler) executeHandler(task *Task, ctx context.Context) bool {
	done := make(chan error, 1)
	go func() {
		var err error
		if recovered := SafeCall(task.ID, func() {
			task.Handler(ctx)
		}, s.panicHandler); recovered {
			err = fmt.Errorf("panic recovered in task %s", task.ID)
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil && s.logger != nil {
			s.logger.Errorf("Task %s failed: %v", task.ID, err)
			return false
		}
		return true
	case <-ctx.Done():
		if s.logger != nil {
			s.logger.Errorf("Task %s timed out", task.ID)
		}
		return false
	}
}

// executeJobInterface 执行任务接口
func (s *scheduler) executeJobInterface(task *Task, ctx context.Context) bool {
	done := make(chan error, 1)
	go func() {
		var err error
		if recovered := SafeCall(task.ID, func() {
			err = task.Job.Run(ctx)
		}, s.panicHandler); recovered {
			err = fmt.Errorf("panic recovered in task %s", task.ID)
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil && s.logger != nil {
			s.logger.Errorf("Task %s failed: %v", task.ID, err)
			return false
		}
		return true
	case <-ctx.Done():
		if s.logger != nil {
			s.logger.Errorf("Task %s timed out", task.ID)
		}
		return false
	}
}
