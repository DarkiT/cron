package cron

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/darkit/cron/history"
)

func waitForCondition(timeout, interval time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(interval)
	}
	return cond()
}

func TestScheduleValidatesInputAndOptions(t *testing.T) {
	c := New()

	noop := func(ctx context.Context) {}

	tests := []struct {
		name    string
		id      string
		spec    string
		opts    JobOptions
		hasOpts bool
	}{
		{name: "empty-id", id: " ", spec: EverySecond},
		{name: "path-separator-slash", id: "task/child", spec: EverySecond},
		{name: "path-separator-backslash", id: `task\\child`, spec: EverySecond},
		{name: "dot-id", id: ".", spec: EverySecond},
		{name: "dotdot-id", id: "..", spec: EverySecond},
		{name: "empty-spec", id: "task", spec: "   "},
		{name: "negative-timeout", id: "task", spec: EverySecond, opts: JobOptions{Timeout: -time.Second}, hasOpts: true},
		{name: "invalid-retries", id: "task", spec: EverySecond, opts: JobOptions{MaxRetries: -2}, hasOpts: true},
		{name: "invalid-misfire", id: "task", spec: EverySecond, opts: JobOptions{MisfirePolicy: MisfirePolicy("bad")}, hasOpts: true},
		{name: "negative-concurrency", id: "task", spec: EverySecond, opts: JobOptions{MaxConcurrent: -1}, hasOpts: true},
		{name: "negative-max-runs", id: "task", spec: EverySecond, opts: JobOptions{MaxRuns: -1}, hasOpts: true},
	}

	for _, tc := range tests {
		var err error
		if tc.hasOpts {
			err = c.Schedule(tc.id, tc.spec, noop, tc.opts)
		} else {
			err = c.Schedule(tc.id, tc.spec, noop)
		}
		if err == nil {
			t.Fatalf("expected error for case %s", tc.name)
		}
	}
}

func TestRegisterJobRejectsInvalidTaskID(t *testing.T) {
	job := &invalidRegisteredJob{name: "bad/job", schedule: EverySecond}
	if err := RegisterJob(job); err == nil {
		t.Fatal("expected RegisterJob to reject invalid task id")
	}

	reg := NewJobRegistry()
	if err := reg.register(&invalidRegisteredJob{name: "..", schedule: EverySecond}); err == nil {
		t.Fatal("expected registry to reject dot-dot task id")
	}
}

func TestAPIsRejectMultipleJobOptions(t *testing.T) {
	c := New()
	noop := func(ctx context.Context) {}

	if err := c.Schedule("too-many-schedule", EverySecond, noop, JobOptions{}, JobOptions{}); err == nil {
		t.Fatal("Schedule should reject multiple JobOptions")
	}
	if err := c.ScheduleJob("too-many-schedule-job", EverySecond, &invalidRegisteredJob{name: "valid-job", schedule: EverySecond}, JobOptions{}, JobOptions{}); err == nil {
		t.Fatal("ScheduleJob should reject multiple JobOptions")
	}
	if err := c.Update("missing", EverySecond, JobOptions{}, JobOptions{}); err == nil {
		t.Fatal("Update should reject multiple JobOptions before task lookup")
	}

	if err := c.ScheduleRegistered(JobOptions{}, JobOptions{}); err == nil {
		t.Fatal("ScheduleRegistered should reject multiple JobOptions")
	}
	if err := c.ScheduleFromRegistry(NewJobRegistry(), JobOptions{}, JobOptions{}); err == nil {
		t.Fatal("ScheduleFromRegistry should reject multiple JobOptions")
	}
}

type invalidRegisteredJob struct {
	name     string
	schedule string
}

func (j *invalidRegisteredJob) Name() string     { return j.name }
func (j *invalidRegisteredJob) Schedule() string { return j.schedule }
func (j *invalidRegisteredJob) Run(ctx context.Context) error {
	return nil
}

type stubRecorder struct {
	closeCalls int32
}

func (r *stubRecorder) Record(taskID string, startTime, endTime time.Time, success bool, retryCount int, err error) {
}

func (r *stubRecorder) Query(filter history.RecordFilter) ([]*history.ExecutionRecord, error) {
	return nil, nil
}

func (r *stubRecorder) Count(filter history.RecordFilter) (int, error) {
	return 0, nil
}

func (r *stubRecorder) Cleanup(before time.Time) (int, error) {
	return 0, nil
}

func (r *stubRecorder) Close() error {
	atomic.AddInt32(&r.closeCalls, 1)
	return nil
}

func TestUpdateRefreshesLabelsAndConcurrencyLimiter(t *testing.T) {
	c := New()
	executed := make(chan struct{}, 1)

	err := c.Schedule("updatable", EverySecond, func(ctx context.Context) {
		select {
		case executed <- struct{}{}:
		default:
		}
	}, JobOptions{
		MaxConcurrent: 0,
		Labels:        map[string]string{"version": "v1"},
	})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	err = c.Update("updatable", EverySecond, JobOptions{
		MaxConcurrent: 1,
		Labels:        map[string]string{"version": "v2"},
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if err := c.RunNow("updatable"); err != nil {
		t.Fatalf("run now failed: %v", err)
	}

	select {
	case <-executed:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("task should execute after update MaxConcurrent from 0 to 1")
	}

	task, ok := c.GetTask("updatable")
	if !ok {
		t.Fatal("expected task to exist")
	}
	if task.Labels["version"] != "v2" {
		t.Fatalf("labels not updated, got %v", task.Labels)
	}
}

type failOnceJob struct {
	executions int64
}

func (j *failOnceJob) Name() string {
	return "fail-once-job"
}

func (j *failOnceJob) Run(ctx context.Context) error {
	if atomic.AddInt64(&j.executions, 1) == 1 {
		return errors.New("first run fails")
	}
	return nil
}

func TestCircuitPauseAutoResume(t *testing.T) {
	c := New()
	defer c.Stop()

	err := c.ScheduleJob("auto-resume", "@every 50ms", &failOnceJob{}, JobOptions{
		MaxRetries:    1,
		RetryInterval: 10 * time.Millisecond,
		FailThreshold: 1,
		PauseDuration: 120 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("schedule job failed: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	pausedObserved := waitForCondition(2*time.Second, 10*time.Millisecond, func() bool {
		info, ok := c.GetTask("auto-resume")
		return ok && info.IsPaused
	})
	if !pausedObserved {
		t.Fatal("expected task to enter paused state")
	}

	resumedObserved := waitForCondition(2*time.Second, 10*time.Millisecond, func() bool {
		info, ok := c.GetTask("auto-resume")
		return ok && !info.IsPaused
	})
	if !resumedObserved {
		t.Fatal("expected task to auto resume after pause duration")
	}
}

func TestStatsLabelsAreIsolatedFromExternalMutation(t *testing.T) {
	c := New()

	labels := map[string]string{"env": "prod"}
	if err := c.Schedule("label-task", EveryMinute, func(ctx context.Context) {}, JobOptions{Labels: labels}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	labels["env"] = "mutated-outside"

	stats, ok := c.GetStats("label-task")
	if !ok {
		t.Fatal("expected stats to exist")
	}
	if stats.Labels["env"] != "prod" {
		t.Fatalf("stats labels should be immutable from caller, got %v", stats.Labels)
	}

	stats.Labels["env"] = "mutated-from-getstats"
	stats2, ok := c.GetStats("label-task")
	if !ok {
		t.Fatal("expected stats to exist on second read")
	}
	if stats2.Labels["env"] != "prod" {
		t.Fatalf("GetStats should return deep copy, got %v", stats2.Labels)
	}

	all := c.GetAllStats()
	all["label-task"].Labels["env"] = "mutated-from-getall"
	stats3, ok := c.GetStats("label-task")
	if !ok {
		t.Fatal("expected stats to exist on third read")
	}
	if stats3.Labels["env"] != "prod" {
		t.Fatalf("GetAllStats should return deep copy, got %v", stats3.Labels)
	}
}

func TestWithLoggerNilKeepsDefaultLogger(t *testing.T) {
	c := New(WithLogger(nil))
	if err := c.Schedule("nil-logger-task", EverySecond, func(ctx context.Context) {}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	c.Stop()
}

type alwaysFailJob struct{}

func (j *alwaysFailJob) Name() string {
	return "always-fail-job"
}

func (j *alwaysFailJob) Run(ctx context.Context) error {
	return errors.New("always fail")
}

func TestCircuitPauseWorksOnFinalFailure(t *testing.T) {
	c := New()
	defer c.Stop()

	err := c.ScheduleJob("pause-on-final-fail", "@every 50ms", &alwaysFailJob{}, JobOptions{
		MaxRetries:    0,
		FailThreshold: 1,
		PauseDuration: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("schedule job failed: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	pausedObserved := waitForCondition(2*time.Second, 10*time.Millisecond, func() bool {
		info, ok := c.GetTask("pause-on-final-fail")
		return ok && info.IsPaused
	})
	if !pausedObserved {
		t.Fatal("expected task to pause after final failure")
	}
}

func TestScheduleInvalidSpecDoesNotPolluteMonitor(t *testing.T) {
	c := New()

	err := c.Schedule("bad-spec-task", "not-a-cron-spec", func(ctx context.Context) {})
	if err == nil {
		t.Fatal("expected invalid schedule error")
	}

	if _, ok := c.GetStats("bad-spec-task"); ok {
		t.Fatal("invalid schedule should not create monitor stats")
	}
}

func TestTaskControlAPIsNormalizeID(t *testing.T) {
	c := New()
	defer c.Stop()

	err := c.Schedule("trim-id", EverySecond, func(ctx context.Context) {})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	if _, err := c.NextRun("  trim-id  "); err != nil {
		t.Fatalf("NextRun with trimmed id failed: %v", err)
	}
	if err := c.RunNow("  trim-id  "); err != nil {
		t.Fatalf("RunNow with trimmed id failed: %v", err)
	}
	if err := c.Update("  trim-id  ", EveryMinute); err != nil {
		t.Fatalf("Update with trimmed id failed: %v", err)
	}
	if err := c.Pause("  trim-id  "); err != nil {
		t.Fatalf("Pause with trimmed id failed: %v", err)
	}
	if err := c.Resume("  trim-id  "); err != nil {
		t.Fatalf("Resume with trimmed id failed: %v", err)
	}

	if task, ok := c.GetTask("  trim-id  "); !ok || task.ID != "trim-id" {
		t.Fatalf("GetTask with trimmed id failed, task=%v, ok=%v", task, ok)
	}

	if err := c.Remove("  trim-id  "); err != nil {
		t.Fatalf("Remove with trimmed id failed: %v", err)
	}

	if _, ok := c.GetTask("trim-id"); ok {
		t.Fatal("task should be removed")
	}
}

func TestTaskControlAPIsRejectEmptyID(t *testing.T) {
	c := New()

	if _, err := c.NextRun("   "); err == nil {
		t.Fatal("NextRun should reject empty id")
	}
	if err := c.RunNow("   "); err == nil {
		t.Fatal("RunNow should reject empty id")
	}
	if err := c.Update("   ", EverySecond); err == nil {
		t.Fatal("Update should reject empty id")
	}
	if err := c.Pause("   "); err == nil {
		t.Fatal("Pause should reject empty id")
	}
	if err := c.Resume("   "); err == nil {
		t.Fatal("Resume should reject empty id")
	}
	if err := c.Remove("   "); err == nil {
		t.Fatal("Remove should reject empty id")
	}
	if task, ok := c.GetTask("   "); ok || task != nil {
		t.Fatalf("GetTask should reject empty id, got task=%v ok=%v", task, ok)
	}
}

func TestRegisterJobNilReturnsError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterJob(nil) should not panic: %v", r)
		}
	}()

	if err := RegisterJob(nil); err == nil {
		t.Fatal("expected RegisterJob(nil) to return error")
	}
}

func TestScheduleRejectsExpiredFinitePlan(t *testing.T) {
	c := New()

	err := c.Schedule("expired-finite-task", "@every 100ms", func(ctx context.Context) {}, JobOptions{
		StartAt: time.Now().Add(-350 * time.Millisecond),
		MaxRuns: 2,
	})
	if err == nil {
		t.Fatal("expected expired finite plan to be rejected")
	}

	if _, ok := c.GetTask("expired-finite-task"); ok {
		t.Fatal("expired finite task should not be added")
	}
}

func TestFiniteTaskStartAtAndAutoRemove(t *testing.T) {
	c := New()
	defer c.Stop()

	var runs int64
	startAt := time.Now().Add(120 * time.Millisecond)

	err := c.Schedule("finite-task", "@every 80ms", func(ctx context.Context) {
		atomic.AddInt64(&runs, 1)
	}, JobOptions{
		StartAt: startAt,
		MaxRuns: 3,
	})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	task, ok := c.GetTask("finite-task")
	if !ok {
		t.Fatal("expected finite task to exist before start")
	}
	if task.RemainingRuns != 3 {
		t.Fatalf("RemainingRuns = %d; want 3", task.RemainingRuns)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	time.Sleep(60 * time.Millisecond)
	if got := atomic.LoadInt64(&runs); got != 0 {
		t.Fatalf("task should not run before StartAt, got %d runs", got)
	}

	removed := waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		_, ok := c.GetTask("finite-task")
		return !ok
	})
	if !removed {
		t.Fatal("expected finite task to be auto removed after max runs")
	}

	if got := atomic.LoadInt64(&runs); got != 3 {
		t.Fatalf("runs = %d; want 3", got)
	}
}

func TestGetTaskReportsRunningState(t *testing.T) {
	c := New()
	defer c.Stop()

	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	err := c.Schedule("running-state", "@every 200ms", func(ctx context.Context) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	}, JobOptions{
		Async:   true,
		StartAt: time.Now().Add(20 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start in time")
	}

	info, ok := c.GetTask("running-state")
	if !ok {
		t.Fatal("expected task info while task is running")
	}
	if !info.IsRunning {
		t.Fatal("expected IsRunning to be true while handler is blocked")
	}

	close(release)

	stopped := waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		info, ok := c.GetTask("running-state")
		return ok && !info.IsRunning
	})
	if !stopped {
		t.Fatal("expected IsRunning to return false after handler exits")
	}
}

func TestFiniteAsyncTaskLastRunCompletesBeforeAutoRemoval(t *testing.T) {
	c := New()
	defer c.Stop()

	var completed atomic.Bool
	var cancelled atomic.Bool

	err := c.Schedule("finite-async", "@every 50ms", func(ctx context.Context) {
		select {
		case <-ctx.Done():
			cancelled.Store(true)
		case <-time.After(120 * time.Millisecond):
			completed.Store(true)
		}
	}, JobOptions{
		Async:   true,
		StartAt: time.Now().Add(20 * time.Millisecond),
		MaxRuns: 1,
	})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	removed := waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		_, ok := c.GetTask("finite-async")
		return !ok
	})
	if !removed {
		t.Fatal("expected finite async task to be auto removed")
	}

	done := waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		return completed.Load()
	})
	if !done {
		t.Fatal("expected final async execution to complete before context cancellation")
	}
	if cancelled.Load() {
		t.Fatal("final async execution should not be cancelled by auto removal")
	}
}

func TestCloseStopsSchedulerAndClosesRecorder(t *testing.T) {
	recorder := &stubRecorder{}
	c := New(WithHistoryRecorder(recorder))

	if err := c.Schedule("close-task", EverySecond, func(ctx context.Context) {}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}

	if calls := atomic.LoadInt32(&recorder.closeCalls); calls != 1 {
		t.Fatalf("recorder close calls = %d; want 1", calls)
	}
	if c.IsRunning() {
		t.Fatal("scheduler should not be running after Close")
	}
	if err := c.Start(); err == nil {
		t.Fatal("Start should reject closed scheduler")
	}
	if err := c.Schedule("closed-task", EverySecond, func(ctx context.Context) {}); err == nil {
		t.Fatal("Schedule should reject closed scheduler")
	}
}

func TestScheduleOnceAtRunsOnceAndRemovesTask(t *testing.T) {
	c := New()
	defer c.Stop()

	var runs int64
	runAt := time.Now().Add(80 * time.Millisecond)

	if err := c.ScheduleOnceAt("once-task", runAt, func(ctx context.Context) {
		atomic.AddInt64(&runs, 1)
	}); err != nil {
		t.Fatalf("schedule once failed: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	removed := waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		_, ok := c.GetTask("once-task")
		return !ok
	})
	if !removed {
		t.Fatal("expected once task to be removed after execution")
	}
	if got := atomic.LoadInt64(&runs); got != 1 {
		t.Fatalf("runs = %d; want 1", got)
	}
}

func TestScheduleLimitedFromHonorsHelperOptions(t *testing.T) {
	c := New()
	defer c.Stop()

	var runs int64
	startAt := time.Now().Add(120 * time.Millisecond)

	if err := c.ScheduleLimitedFrom("limited-helper", "@every 70ms", startAt, 2, func(ctx context.Context) {
		atomic.AddInt64(&runs, 1)
	}); err != nil {
		t.Fatalf("schedule limited failed: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	time.Sleep(60 * time.Millisecond)
	if got := atomic.LoadInt64(&runs); got != 0 {
		t.Fatalf("task should not run before helper startAt, got %d", got)
	}

	removed := waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		_, ok := c.GetTask("limited-helper")
		return !ok
	})
	if !removed {
		t.Fatal("expected helper task to be auto removed after max runs")
	}
	if got := atomic.LoadInt64(&runs); got != 2 {
		t.Fatalf("runs = %d; want 2", got)
	}
}

func TestHelperAPIsRejectConflictingReservedOptions(t *testing.T) {
	c := New()

	if err := c.ScheduleOnceAt("conflict-once", time.Now().Add(time.Second), func(ctx context.Context) {}, JobOptions{MaxRuns: 2}); err == nil {
		t.Fatal("ScheduleOnceAt should reject conflicting MaxRuns")
	}

	if err := c.ScheduleLimitedFrom("conflict-limited", EverySecond, time.Now().Add(time.Second), 2, func(ctx context.Context) {}, JobOptions{
		StartAt: time.Now().Add(2 * time.Second),
	}); err == nil {
		t.Fatal("ScheduleLimitedFrom should reject conflicting StartAt")
	}
}
