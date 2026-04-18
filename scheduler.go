package cron

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/darkit/cron/history"
	"github.com/darkit/cron/internal/parser"
)

// scheduler 是核心调度器
type scheduler struct {
	tasks        map[string]*taskRunner
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	execWG       sync.WaitGroup // 跟踪每次任务执行，确保优雅停止时能等待
	running      bool
	logger       Logger
	monitor      *Monitor
	panicHandler PanicHandler
	rootCtx      context.Context
	recorder     history.Recorder // 历史记录器（可选）
	eventHook    EventHook
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

// parseSchedule 解析 cron 表达式，兼容5段与6段格式
func parseSchedule(spec string) (parser.Schedule, error) {
	fields := strings.Fields(strings.TrimSpace(spec))
	specFields := len(fields)

	if specFields == 5 {
		return parser.ParseStandard(spec)
	}

	p := parser.NewParser(parser.Second | parser.Minute | parser.Hour | parser.Dom | parser.Month | parser.Dow | parser.Descriptor)
	return p.Parse(spec)
}

// resetFailure 重置失败计数
func (r *taskRunner) resetFailure() {
	r.failure.mu.Lock()
	r.failure.count = 0
	r.failure.windowStart = time.Time{}
	r.failure.mu.Unlock()
}

// recordFailure 记录一次失败并在达到阈值时自动暂停
func (r *taskRunner) recordFailure(window time.Duration, threshold int, pauseDuration time.Duration, logger Logger, taskID string) {
	if threshold <= 0 {
		return
	}

	now := time.Now()
	r.failure.mu.Lock()
	defer r.failure.mu.Unlock()

	if window > 0 {
		if r.failure.windowStart.IsZero() || now.Sub(r.failure.windowStart) > window {
			r.failure.windowStart = now
			r.failure.count = 0
		}
	}

	r.failure.count++
	if r.failure.count >= threshold {
		r.failure.count = 0
		r.failure.windowStart = now

		r.mu.Lock()
		r.paused = true
		r.pauseUntil = now.Add(pauseDuration)
		r.nextRun = r.pauseUntil
		r.mu.Unlock()

		if logger != nil {
			logger.Warnf("Task %s paused for %v after %d failures", taskID, pauseDuration, threshold)
		}
	}
}

// taskRunner 运行任务的实体
type taskRunner struct {
	task          *Task
	schedule      parser.Schedule
	nextRun       time.Time
	running       bool
	activeRuns    int
	paused        bool
	pauseUntil    time.Time // 自动暂停到期时间，零值表示手动暂停
	remainingRuns int
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.RWMutex
	semaphore     chan struct{} // 并发控制

	// 重试状态（线程安全）
	retry struct {
		mu       sync.Mutex
		attempts int // 当前重试次数
	}

	failure struct {
		mu          sync.Mutex
		count       int
		windowStart time.Time
	}
}

// cloneJobOptions 复制 JobOptions，确保 map 不被外部修改
func cloneJobOptions(opts JobOptions) JobOptions {
	cloned := opts
	if opts.Labels != nil {
		cloned.Labels = make(map[string]string, len(opts.Labels))
		for k, v := range opts.Labels {
			cloned.Labels[k] = v
		}
	}
	return cloned
}

// cloneLabels 复制标签 map，避免外部修改内部状态
func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for k, v := range labels {
		cloned[k] = v
	}
	return cloned
}

// remainingRunsFromOptions 将公开配置转换为内部剩余执行次数表示。
func remainingRunsFromOptions(opts JobOptions) int {
	if opts.MaxRuns <= 0 {
		return -1
	}
	return opts.MaxRuns
}

// nextOccurrenceOnOrAfter 返回不早于指定时间的下一个触发点。
func nextOccurrenceOnOrAfter(schedule parser.Schedule, from time.Time) time.Time {
	if _, ok := schedule.(*parser.ConstantDelaySchedule); ok {
		return from
	}

	candidate := schedule.Next(from.Add(-time.Second))
	if candidate.Before(from) {
		return schedule.Next(from)
	}
	return candidate
}

// defaultNextRun 计算未指定 StartAt 时的首次触发点。
func defaultNextRun(schedule parser.Schedule, now time.Time) time.Time {
	if _, ok := schedule.(*parser.ConstantDelaySchedule); ok {
		return now
	}
	return schedule.Next(now)
}

// planInitialState 计算任务初次加入调度器时的运行状态。
func planInitialState(schedule parser.Schedule, opts JobOptions, now time.Time) (time.Time, int, bool) {
	remainingRuns := remainingRunsFromOptions(opts)
	if opts.StartAt.IsZero() {
		return defaultNextRun(schedule, now), remainingRuns, false
	}

	if delaySchedule, ok := schedule.(*parser.ConstantDelaySchedule); ok {
		nextRun := opts.StartAt
		if !opts.StartAt.After(now) {
			delay := delaySchedule.Delay
			if delay <= 0 {
				return time.Time{}, 0, true
			}

			elapsed := now.Sub(opts.StartAt)
			steps := int(elapsed / delay)
			nextRun = opts.StartAt.Add(time.Duration(steps) * delay)
			if nextRun.Before(now) {
				steps++
				nextRun = nextRun.Add(delay)
			}

			if remainingRuns > 0 {
				if steps >= remainingRuns {
					return time.Time{}, 0, true
				}
				remainingRuns -= steps
			}
		}
		return nextRun, remainingRuns, false
	}

	firstRun := nextOccurrenceOnOrAfter(schedule, opts.StartAt)
	if remainingRuns < 0 {
		if firstRun.Before(now) {
			return nextOccurrenceOnOrAfter(schedule, now), remainingRuns, false
		}
		return firstRun, remainingRuns, false
	}

	nextRun := firstRun
	for nextRun.Before(now) {
		remainingRuns--
		if remainingRuns == 0 {
			return time.Time{}, 0, true
		}
		nextRun = schedule.Next(nextRun)
		if nextRun.IsZero() {
			return time.Time{}, 0, true
		}
	}

	return nextRun, remainingRuns, false
}

// recomputeNextRun 重新计算任务恢复后的下次触发时间。
func recomputeNextRun(schedule parser.Schedule, startAt time.Time, remainingRuns int, now time.Time) (time.Time, bool) {
	if remainingRuns == 0 {
		return time.Time{}, true
	}
	if !startAt.IsZero() {
		if delaySchedule, ok := schedule.(*parser.ConstantDelaySchedule); ok {
			nextRun := startAt
			if !startAt.After(now) {
				delay := delaySchedule.Delay
				if delay <= 0 {
					return time.Time{}, true
				}
				elapsed := now.Sub(startAt)
				steps := int(elapsed / delay)
				nextRun = startAt.Add(time.Duration(steps) * delay)
				if nextRun.Before(now) {
					nextRun = nextRun.Add(delay)
				}
			}
			return nextRun, nextRun.IsZero()
		}

		if startAt.After(now) {
			nextRun := nextOccurrenceOnOrAfter(schedule, startAt)
			return nextRun, nextRun.IsZero()
		}
	}

	nextRun := nextOccurrenceOnOrAfter(schedule, now)
	return nextRun, nextRun.IsZero()
}

// advancePlanAfterTrigger 根据当前已触发的计划点推进任务状态。
func (s *scheduler) advancePlanAfterTrigger(runner *taskRunner, triggerTime time.Time) (time.Time, int, bool) {
	runner.mu.RLock()
	schedule := runner.schedule
	remainingRuns := runner.remainingRuns
	options := cloneJobOptions(runner.task.Options)
	runner.mu.RUnlock()

	consume := func() bool {
		if remainingRuns < 0 {
			return false
		}
		if remainingRuns == 0 {
			return true
		}
		remainingRuns--
		return remainingRuns == 0
	}

	if consume() {
		return time.Time{}, 0, true
	}

	nextRun := schedule.Next(triggerTime)
	if nextRun.IsZero() {
		return time.Time{}, remainingRuns, true
	}

	now := time.Now()
	switch options.MisfirePolicy {
	case MisfireCatchUp:
		maxCatchUp := options.MaxCatchUp
		if maxCatchUp <= 0 {
			maxCatchUp = 5
		}

		catchUps := 0
		for nextRun.Before(now) {
			if catchUps < maxCatchUp {
				s.executeTask(runner)
				if consume() {
					return time.Time{}, 0, true
				}
				nextRun = schedule.Next(nextRun)
				if nextRun.IsZero() {
					return time.Time{}, remainingRuns, true
				}
				catchUps++
				continue
			}

			if consume() {
				return time.Time{}, 0, true
			}
			nextRun = schedule.Next(nextRun)
			if nextRun.IsZero() {
				return time.Time{}, remainingRuns, true
			}
		}
	default:
		for nextRun.Before(now) {
			if consume() {
				return time.Time{}, 0, true
			}
			nextRun = schedule.Next(nextRun)
			if nextRun.IsZero() {
				return time.Time{}, remainingRuns, true
			}
		}
	}

	return nextRun, remainingRuns, false
}

// addTask 添加一个任务
func (s *scheduler) addTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[task.ID]; exists {
		return fmt.Errorf("task %s already exists", task.ID)
	}

	// 解析cron表达式
	schedule, err := parseSchedule(task.Schedule)
	if err != nil {
		return fmt.Errorf("invalid cron spec %s: %w", task.Schedule, err)
	}

	// 创建任务运行器
	ctx, cancel := context.WithCancel(s.ctx)
	now := time.Now()
	next, remainingRuns, expired := planInitialState(schedule, task.Options, now)
	if expired {
		cancel()
		return fmt.Errorf("task %s schedule already expired", task.ID)
	}

	runner := &taskRunner{
		task:          task,
		schedule:      schedule,
		nextRun:       next,
		remainingRuns: remainingRuns,
		ctx:           ctx,
		cancel:        cancel,
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

// expireTask 在计划执行完毕或计划窗口过期后自动清理任务。
func (s *scheduler) expireTask(id string) {
	s.mu.Lock()
	runner, exists := s.tasks[id]
	if !exists {
		s.mu.Unlock()
		return
	}

	runner.mu.RLock()
	activeRuns := runner.activeRuns
	runner.mu.RUnlock()

	if activeRuns == 0 && runner.cancel != nil {
		runner.cancel()
	}
	delete(s.tasks, id)
	s.mu.Unlock()

	if s.monitor != nil {
		s.monitor.removeTask(id)
	}
	if s.logger != nil {
		s.logger.Infof("Task %s expired and removed automatically", id)
	}
}

// updateTask 更新任务的调度表达式和可选配置
func (s *scheduler) updateTask(id, schedule string, opts *JobOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	runner, exists := s.tasks[id]
	if !exists {
		return fmt.Errorf("task %s not found", id)
	}

	parsed, err := parseSchedule(schedule)
	if err != nil {
		return fmt.Errorf("invalid cron spec %s: %w", schedule, err)
	}

	now := time.Now()

	runner.mu.Lock()
	currentRemainingRuns := runner.remainingRuns
	currentOptions := cloneJobOptions(runner.task.Options)
	currentLabels := cloneLabels(runner.task.Labels)
	nextRun := time.Time{}
	remainingRuns := currentRemainingRuns
	if opts != nil {
		var expired bool
		nextRun, remainingRuns, expired = planInitialState(parsed, *opts, now)
		if expired {
			runner.mu.Unlock()
			return fmt.Errorf("task %s schedule already expired", id)
		}
	} else {
		var expired bool
		nextRun, expired = recomputeNextRun(parsed, currentOptions.StartAt, currentRemainingRuns, now)
		if expired {
			runner.mu.Unlock()
			return fmt.Errorf("task %s schedule already expired", id)
		}
	}

	runner.schedule = parsed
	runner.task.Schedule = schedule
	runner.remainingRuns = remainingRuns
	runner.nextRun = nextRun
	if opts != nil {
		runner.task.Options = cloneJobOptions(*opts)
		runner.task.Labels = cloneLabels(runner.task.Options.Labels)
		if runner.task.Options.MaxConcurrent > 0 {
			runner.semaphore = make(chan struct{}, runner.task.Options.MaxConcurrent)
		} else {
			runner.semaphore = nil
		}
	}
	labels := currentLabels
	if opts != nil {
		labels = cloneLabels(runner.task.Labels)
	}
	misfirePolicy := string(runner.task.Options.MisfirePolicy)
	runner.mu.Unlock()

	if s.monitor != nil {
		s.monitor.updateSchedule(id, schedule)
		if opts != nil {
			s.monitor.updateTaskMeta(id, labels, misfirePolicy)
		}
	}

	return nil
}

// pauseTask 暂停任务调度
func (s *scheduler) pauseTask(id string) error {
	s.mu.Lock()
	runner, exists := s.tasks[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", id)
	}

	runner.mu.Lock()
	runner.paused = true
	runner.pauseUntil = time.Time{}
	runner.mu.Unlock()
	s.mu.Unlock()

	if s.monitor != nil {
		s.monitor.setPauseUntil(id, time.Now())
	}
	return nil
}

// resumeTask 恢复任务调度
func (s *scheduler) resumeTask(id string) error {
	s.mu.Lock()
	runner, exists := s.tasks[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", id)
	}

	runner.mu.Lock()
	runner.paused = false
	runner.pauseUntil = time.Time{}
	nextRun, expired := recomputeNextRun(runner.schedule, runner.task.Options.StartAt, runner.remainingRuns, time.Now())
	if expired {
		runner.mu.Unlock()
		s.mu.Unlock()
		s.expireTask(id)
		return nil
	}
	runner.nextRun = nextRun
	runner.mu.Unlock()
	s.mu.Unlock()

	if s.monitor != nil {
		s.monitor.setPauseUntil(id, time.Time{})
	}
	return nil
}

// runNow 立即触发任务一次，独立于调度计划
func (s *scheduler) runNow(id string) error {
	s.mu.RLock()
	runner, exists := s.tasks[id]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("task %s not found", id)
	}

	if s.logger != nil {
		s.logger.Infof("Trigger task %s to run immediately", id)
	}

	go s.executeTask(runner)
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
	s.stopWithTimeout(5 * time.Second)
}

// stopWithTimeout 停止调度器，可等待正在执行的任务
func (s *scheduler) stopWithTimeout(timeout time.Duration) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}

	s.running = false
	s.cancel()
	s.mu.Unlock()

	// 等待调度循环退出
	s.wg.Wait()

	// 等待正在执行的任务完成，避免异步任务泄漏
	s.waitExecutions(timeout)

	// 重新构建上下文，允许后续重新启动
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(s.rootCtx)
	for _, runner := range s.tasks {
		// 先 cancel 旧 context，避免 goroutine 泄露
		if runner.cancel != nil {
			runner.cancel()
		}
		runnerCtx, cancel := context.WithCancel(s.ctx)
		runner.ctx = runnerCtx
		runner.cancel = cancel
		runner.mu.Lock()
		runner.activeRuns = 0
		runner.running = false
		nextRun, expired := recomputeNextRun(runner.schedule, runner.task.Options.StartAt, runner.remainingRuns, time.Now())
		if expired {
			runner.nextRun = time.Time{}
		} else {
			runner.nextRun = nextRun
		}
		runner.mu.Unlock()
	}
	s.mu.Unlock()
}

// waitExecutions 等待正在执行的任务完成，超时则提前返回
func (s *scheduler) waitExecutions(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		s.execWG.Wait()
		close(done)
	}()

	if timeout <= 0 {
		<-done
		return
	}

	select {
	case <-done:
	case <-time.After(timeout):
		if s.logger != nil {
			s.logger.Warnf("Stop waiting for running tasks timed out after %v", timeout)
		}
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

// getTaskInfo 获取指定任务的详细信息
func (s *scheduler) getTaskInfo(id string) (*TaskInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runner, exists := s.tasks[id]
	if !exists {
		return nil, false
	}

	runner.mu.RLock()
	defer runner.mu.RUnlock()

	return &TaskInfo{
		ID:            runner.task.ID,
		Schedule:      runner.task.Schedule,
		Options:       cloneJobOptions(runner.task.Options),
		Labels:        cloneLabels(runner.task.Labels),
		NextRun:       runner.nextRun,
		RemainingRuns: runner.remainingRuns,
		IsPaused:      runner.paused,
		IsRunning:     runner.running,
		CreatedAt:     runner.task.created,
	}, true
}

// getAllTaskInfo 获取所有任务的详细信息
func (s *scheduler) getAllTaskInfo() []*TaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*TaskInfo, 0, len(s.tasks))
	for _, runner := range s.tasks {
		runner.mu.RLock()
		info := &TaskInfo{
			ID:            runner.task.ID,
			Schedule:      runner.task.Schedule,
			Options:       cloneJobOptions(runner.task.Options),
			Labels:        cloneLabels(runner.task.Labels),
			NextRun:       runner.nextRun,
			RemainingRuns: runner.remainingRuns,
			IsPaused:      runner.paused,
			IsRunning:     runner.running,
			CreatedAt:     runner.task.created,
		}
		runner.mu.RUnlock()
		result = append(result, info)
	}
	return result
}

// pauseAll 暂停所有任务
func (s *scheduler) pauseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, runner := range s.tasks {
		runner.mu.Lock()
		runner.paused = true
		runner.pauseUntil = time.Time{}
		runner.mu.Unlock()

		if s.monitor != nil {
			s.monitor.setPauseUntil(id, time.Now())
		}
	}
}

// resumeAll 恢复所有任务
func (s *scheduler) resumeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, runner := range s.tasks {
		runner.mu.Lock()
		runner.paused = false
		runner.pauseUntil = time.Time{}
		nextRun, expired := recomputeNextRun(runner.schedule, runner.task.Options.StartAt, runner.remainingRuns, time.Now())
		if expired {
			runner.nextRun = time.Time{}
		} else {
			runner.nextRun = nextRun
		}
		runner.mu.Unlock()

		if s.monitor != nil {
			s.monitor.setPauseUntil(id, time.Time{})
		}
	}
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
		runner.mu.RLock()
		next := runner.nextRun
		runner.mu.RUnlock()
		if next.IsZero() {
			s.expireTask(runner.task.ID)
			return
		}

		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}

		select {
		case <-runner.ctx.Done():
			return
		case <-time.After(wait):
			runner.mu.RLock()
			paused := runner.paused
			pauseUntil := runner.pauseUntil
			schedule := runner.schedule
			currentNext := runner.nextRun
			taskID := runner.task.ID
			runner.mu.RUnlock()
			if paused {
				now := time.Now()
				if !pauseUntil.IsZero() && !now.Before(pauseUntil) {
					runner.mu.Lock()
					nextRun, expired := recomputeNextRun(schedule, runner.task.Options.StartAt, runner.remainingRuns, now)
					if expired {
						runner.mu.Unlock()
						s.expireTask(taskID)
						return
					}
					runner.paused = false
					runner.pauseUntil = time.Time{}
					runner.nextRun = nextRun
					runner.mu.Unlock()
					if s.monitor != nil {
						s.monitor.setPauseUntil(taskID, time.Time{})
					}
					continue
				}

				runner.mu.Lock()
				next := schedule.Next(now)
				if !pauseUntil.IsZero() && pauseUntil.Before(next) {
					next = pauseUntil
				}
				runner.nextRun = next
				runner.mu.Unlock()
				continue
			}
			s.executeTask(runner)

			nextRun, remainingRuns, expired := s.advancePlanAfterTrigger(runner, currentNext)
			if expired {
				s.expireTask(taskID)
				return
			}

			runner.mu.Lock()
			// 如果调度配置被并发更新，只在当前计划点未变化时推进内部状态。
			if runner.nextRun.Equal(currentNext) && runner.schedule == schedule {
				runner.remainingRuns = remainingRuns
				runner.nextRun = nextRun
			}
			runner.mu.Unlock()
		}
	}
}

// executeTaskJobOnce 执行任务一次（新增 helper）
// 返回：success bool（是否成功），err error（失败原因）
func (s *scheduler) executeTaskJobOnce(task *Task, ctx context.Context) (bool, error) {
	var (
		success bool
		err     error
	)

	// 执行任务
	if task.Handler != nil {
		success, err = s.executeHandler(task, ctx)
	} else if task.Job != nil {
		success, err = s.executeJobInterface(task, ctx)
	}

	return success, err
}

// runTaskWithRetry 带重试的任务执行包装器
func (s *scheduler) runTaskWithRetry(runner *taskRunner, baseCtx context.Context) {
	runner.mu.RLock()
	task := &Task{
		ID:      runner.task.ID,
		Handler: runner.task.Handler,
		Job:     runner.task.Job,
		Options: cloneJobOptions(runner.task.Options),
	}
	runner.mu.RUnlock()

	maxRetries := task.Options.MaxRetries
	retryInterval := task.Options.RetryInterval
	timeout := task.Options.Timeout
	failThreshold := task.Options.FailThreshold
	failWindow := task.Options.FailWindow
	pauseDuration := task.Options.PauseDuration
	if pauseDuration <= 0 {
		pauseDuration = 30 * time.Second
	}

	startTime := time.Now()
	finalSuccess := false
	actualRetries := 0
	var lastErr error

	if s.eventHook != nil {
		s.eventHook(Event{TaskID: task.ID, Start: startTime})
	}

	defer func() {
		endTime := time.Now()

		// 清理重试状态，避免影响下次调度
		runner.retry.mu.Lock()
		runner.retry.attempts = 0
		runner.retry.mu.Unlock()

		// 记录最终统计
		duration := endTime.Sub(startTime)
		if s.monitor != nil {
			lastError := ""
			if lastErr != nil {
				lastError = lastErr.Error()
			}
			s.monitor.recordExecutionResult(task.ID, duration, finalSuccess, actualRetries, lastError)
		}

		if s.eventHook != nil {
			errMsg := ""
			if lastErr != nil {
				errMsg = lastErr.Error()
			}
			s.eventHook(Event{
				TaskID:   task.ID,
				Start:    startTime,
				End:      endTime,
				Success:  finalSuccess,
				Error:    errMsg,
				Retries:  actualRetries,
				Duration: duration,
			})
		}

		// 记录执行历史（如果启用）
		if s.recorder != nil {
			recordErr := lastErr
			if !finalSuccess && recordErr == nil {
				recordErr = fmt.Errorf("task failed after %d retries", actualRetries)
			}
			s.recorder.Record(task.ID, startTime, endTime, finalSuccess, actualRetries, recordErr)
		}
	}()

	for attempt := 0; maxRetries < 0 || attempt <= maxRetries; attempt++ {
		// 若基础上下文已取消，则直接退出
		select {
		case <-baseCtx.Done():
			if s.logger != nil {
				s.logger.Warnf("Task %s cancelled before attempt %d", task.ID, attempt+1)
			}
			finalSuccess = false
			actualRetries = attempt
			lastErr = baseCtx.Err()
			return
		default:
		}

		// 为当前尝试创建独立的超时上下文，避免前一次的取消影响后续重试
		attemptCtx := baseCtx
		cancelAttempt := func() {}
		if timeout > 0 {
			attemptCtx, cancelAttempt = context.WithTimeout(baseCtx, timeout)
		}

		success, execErr := s.executeTaskJobOnce(task, attemptCtx)
		cancelAttempt()

		if success {
			// 成功，重置重试计数与失败熔断计数
			runner.resetFailure()
			finalSuccess = true
			actualRetries = attempt
			lastErr = nil
			return
		}
		lastErr = execErr

		// 连续失败熔断处理（包含最终失败场景）
		runner.recordFailure(failWindow, failThreshold, pauseDuration, s.logger, task.ID)
		if s.monitor != nil {
			runner.mu.RLock()
			pausedUntil := runner.pauseUntil
			isPaused := runner.paused
			runner.mu.RUnlock()
			if isPaused {
				s.monitor.setPauseUntil(task.ID, pausedUntil)
			}
		}

		// 检查是否达到最大重试次数
		if maxRetries >= 0 && attempt == maxRetries {
			if s.logger != nil {
				if lastErr != nil {
					s.logger.Errorf("Task %s failed after %d retries: %v", task.ID, attempt, lastErr)
				} else {
					s.logger.Errorf("Task %s failed after %d retries", task.ID, attempt)
				}
			}
			finalSuccess = false
			actualRetries = attempt
			return
		}

		// 记录重试状态
		runner.retry.mu.Lock()
		runner.retry.attempts = attempt + 1
		runner.retry.mu.Unlock()

		if s.logger != nil {
			if lastErr != nil {
				s.logger.Warnf("Task %s failed, retrying %d/%d after %v: %v",
					task.ID, attempt+1, maxRetries, retryInterval, lastErr)
			} else {
				s.logger.Warnf("Task %s failed, retrying %d/%d after %v",
					task.ID, attempt+1, maxRetries, retryInterval)
			}
		}

		// 等待重试间隔（检查上下文取消）
		if retryInterval > 0 {
			timer := time.NewTimer(retryInterval)
			select {
			case <-baseCtx.Done():
				timer.Stop()
				finalSuccess = false
				actualRetries = attempt + 1
				lastErr = baseCtx.Err()
				return
			case <-timer.C:
			}
		} else {
			// 立即重试，但仍需检查上下文
			select {
			case <-baseCtx.Done():
				finalSuccess = false
				actualRetries = attempt + 1
				lastErr = baseCtx.Err()
				return
			default:
			}
		}
	}
}

// executeTask 执行任务
func (s *scheduler) executeTask(runner *taskRunner) {
	// 并发控制逻辑
	runner.mu.RLock()
	taskID := runner.task.ID
	maxConcurrent := runner.task.Options.MaxConcurrent
	async := runner.task.Options.Async
	runner.mu.RUnlock()

	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			if s.panicHandler != nil {
				s.panicHandler.HandlePanic(taskID, r, stack)
			} else if s.logger != nil {
				s.logger.Errorf("Task %s panicked: %v", taskID, r)
			}
		}
	}()

	release := func() {}

	if maxConcurrent > 0 {
		// MaxConcurrent > 0: 严格限制最大并发数，超过则立即放弃任务
		// 注意：动态更新 MaxConcurrent 时需要重建 semaphore
		runner.mu.Lock()
		if runner.semaphore == nil || cap(runner.semaphore) != maxConcurrent {
			runner.semaphore = make(chan struct{}, maxConcurrent)
		}
		semaphore := runner.semaphore
		runner.mu.Unlock()

		select {
		case semaphore <- struct{}{}:
			// 获得执行权限
			release = func() {
				<-semaphore
			}
		default:
			// 超过并发限制，立即放弃任务
			if s.logger != nil {
				s.logger.Warnf("Task %s skipped due to concurrency limit (%d)", taskID, maxConcurrent)
			}
			if s.monitor != nil {
				s.monitor.recordSkip(taskID)
			}
			return
		}
	}
	// MaxConcurrent = 0: 允许无限并发，不做任何限制

	s.execWG.Add(1)
	runner.mu.Lock()
	runner.activeRuns++
	runner.running = runner.activeRuns > 0
	taskCtx := runner.ctx
	currentlyRunning := runner.running
	runner.mu.Unlock()
	if s.monitor != nil {
		s.monitor.setRunning(taskID, currentlyRunning)
	}
	run := func() {
		defer release()
		defer func() {
			runner.mu.Lock()
			if runner.activeRuns > 0 {
				runner.activeRuns--
			}
			runner.running = runner.activeRuns > 0
			currentlyRunning := runner.running
			runner.mu.Unlock()
			if s.monitor != nil {
				s.monitor.setRunning(taskID, currentlyRunning)
			}
			s.execWG.Done()
		}()

		// 使用重试包装器（关键修改）
		s.runTaskWithRetry(runner, taskCtx)
	}

	if async {
		go run()
	} else {
		run()
	}
}

// executeFunc 定义通用执行函数类型，用于统一处理任务执行逻辑
type executeFunc func() error

// executeWithTimeout 通用执行器，提供 goroutine 监控、超时控制和 panic 恢复
// 参数:
//   - taskID: 任务ID，用于日志记录
//   - ctx: 上下文，控制超时和取消
//   - fn: 实际执行的函数
//   - panicHandler: panic 处理器（可选）
//   - logger: 日志记录器（可选）
//
// 返回值:
//   - success: 是否成功执行
//   - err: 执行错误信息
func (s *scheduler) executeWithTimeout(taskID string, ctx context.Context, fn executeFunc, panicHandler PanicHandler, logger Logger) (bool, error) {
	// 记录执行前的 goroutine 数量
	goroutinesBefore := runtime.NumGoroutine()

	done := make(chan error, 1)

	// 在新 goroutine 中执行任务
	go func() {
		var err error
		// panic 恢复处理
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				if panicHandler != nil {
					panicHandler.HandlePanic(taskID, r, stack)
				} else if logger != nil {
					logger.Errorf("Task %s panicked: %v\n%s", taskID, r, string(stack))
				}
				err = fmt.Errorf("panic recovered in task %s: %v", taskID, r)
			}
			done <- err
		}()

		// 执行实际任务
		err = fn()
	}()

	// 等待任务完成或超时
	select {
	case err := <-done:
		// 记录执行后的 goroutine 数量
		goroutinesAfter := runtime.NumGoroutine()

		// 如果 goroutine 数量增加，记录警告（可能存在泄漏）
		if goroutinesAfter > goroutinesBefore+1 {
			if logger != nil {
				logger.Warnf("Task %s may have goroutine leak: before=%d, after=%d",
					taskID, goroutinesBefore, goroutinesAfter)
			}
		}

		if err != nil {
			if logger != nil {
				logger.Errorf("Task %s failed: %v", taskID, err)
			}
			return false, err
		}
		return true, nil

	case <-ctx.Done():
		// 超时或取消
		timeoutErr := fmt.Errorf("task %s timed out: %w", taskID, ctx.Err())
		if logger != nil {
			logger.Errorf("Task %s timed out: %v", taskID, ctx.Err())
		}
		return false, timeoutErr
	}
}

// executeHandler 执行处理函数
func (s *scheduler) executeHandler(task *Task, ctx context.Context) (bool, error) {
	// 使用通用执行器，包含 goroutine 监控、超时控制和 panic 恢复
	return s.executeWithTimeout(task.ID, ctx, func() error {
		task.Handler(ctx)
		return nil
	}, s.panicHandler, s.logger)
}

// executeJobInterface 执行任务接口
func (s *scheduler) executeJobInterface(task *Task, ctx context.Context) (bool, error) {
	// 使用通用执行器，包含 goroutine 监控、超时控制和 panic 恢复
	return s.executeWithTimeout(task.ID, ctx, func() error {
		return task.Job.Run(ctx)
	}, s.panicHandler, s.logger)
}
