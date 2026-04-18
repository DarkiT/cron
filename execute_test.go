package cron

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestExecuteWithTimeout_Success 测试成功执行的情况
func TestExecuteWithTimeout_Success(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	executed := false
	success, err := s.executeWithTimeout("test-task", context.Background(), func() error {
		executed = true
		return nil
	}, nil, s.logger)

	if !success {
		t.Error("Expected success=true")
	}
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !executed {
		t.Error("Expected task to be executed")
	}
}

// TestExecuteWithTimeout_Error 测试执行返回错误的情况
func TestExecuteWithTimeout_Error(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	expectedErr := errors.New("task error")
	success, err := s.executeWithTimeout("test-task", context.Background(), func() error {
		return expectedErr
	}, nil, s.logger)

	if success {
		t.Error("Expected success=false")
	}
	if err != expectedErr {
		t.Errorf("Expected error=%v, got: %v", expectedErr, err)
	}
}

// TestExecuteWithTimeout_Timeout 测试超时的情况
func TestExecuteWithTimeout_Timeout(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	success, err := s.executeWithTimeout("test-task", ctx, func() error {
		time.Sleep(500 * time.Millisecond) // 执行时间超过超时时间
		return nil
	}, nil, s.logger)

	if success {
		t.Error("Expected success=false on timeout")
	}
	if err == nil {
		t.Error("Expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}
}

// TestExecuteWithTimeout_Panic 测试 panic 恢复
func TestExecuteWithTimeout_Panic(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	panicCalled := false
	s.panicHandler = &testPanicHandler{
		handleFunc: func(taskID string, r interface{}, stack []byte) {
			panicCalled = true
			if taskID != "test-task" {
				t.Errorf("Expected taskID=test-task, got: %s", taskID)
			}
			if r != "test panic" {
				t.Errorf("Expected panic message='test panic', got: %v", r)
			}
		},
	}

	success, err := s.executeWithTimeout("test-task", context.Background(), func() error {
		panic("test panic")
	}, s.panicHandler, s.logger)

	if success {
		t.Error("Expected success=false after panic")
	}
	if err == nil {
		t.Error("Expected error after panic")
	}
	if !panicCalled {
		t.Error("Expected panic handler to be called")
	}
}

// TestExecuteWithTimeout_GoroutineMonitoring 测试 goroutine 监控
func TestExecuteWithTimeout_GoroutineMonitoring(t *testing.T) {
	s := newScheduler()
	logEntries := &logBuffer{}
	s.logger = logEntries

	// 创建一个会泄漏 goroutine 的任务
	// 我们启动多个 goroutine 以确保泄漏能被检测到
	success, err := s.executeWithTimeout("test-task", context.Background(), func() error {
		// 启动多个不会立即退出的 goroutine
		for i := 0; i < 5; i++ {
			go func() {
				time.Sleep(5 * time.Second)
			}()
		}
		// 等待确保所有 goroutine 都已启动
		time.Sleep(100 * time.Millisecond)
		return nil
	}, nil, s.logger)

	if !success {
		t.Error("Expected success=true")
	}
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// 检查是否记录了 goroutine 泄漏警告
	// 由于我们启动了5个额外的 goroutine，应该会触发警告
	if !logEntries.contains("may have goroutine leak") {
		t.Error("Expected goroutine leak warning")
	}
}

// TestExecuteHandler_Success 测试 executeHandler 成功执行
func TestExecuteHandler_Success(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	executed := false
	task := &Task{
		ID: "handler-task",
		Handler: func(ctx context.Context) {
			executed = true
		},
	}

	success, err := s.executeHandler(task, context.Background())

	if !success {
		t.Error("Expected success=true")
	}
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !executed {
		t.Error("Expected handler to be executed")
	}
}

// TestExecuteHandler_Panic 测试 executeHandler panic 恢复
func TestExecuteHandler_Panic(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	panicCalled := false
	s.panicHandler = &testPanicHandler{
		handleFunc: func(taskID string, r interface{}, stack []byte) {
			panicCalled = true
		},
	}

	task := &Task{
		ID: "handler-task",
		Handler: func(ctx context.Context) {
			panic("handler panic")
		},
	}

	success, err := s.executeHandler(task, context.Background())

	if success {
		t.Error("Expected success=false after panic")
	}
	if err == nil {
		t.Error("Expected error after panic")
	}
	if !panicCalled {
		t.Error("Expected panic handler to be called")
	}
}

// TestExecuteJobInterface_Success 测试 executeJobInterface 成功执行
func TestExecuteJobInterface_Success(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	executed := false
	task := &Task{
		ID: "job-task",
		Job: &testJob{
			runFunc: func(ctx context.Context) error {
				executed = true
				return nil
			},
		},
	}

	success, err := s.executeJobInterface(task, context.Background())

	if !success {
		t.Error("Expected success=true")
	}
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !executed {
		t.Error("Expected job to be executed")
	}
}

// TestExecuteJobInterface_Error 测试 executeJobInterface 返回错误
func TestExecuteJobInterface_Error(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	expectedErr := errors.New("job error")
	task := &Task{
		ID: "job-task",
		Job: &testJob{
			runFunc: func(ctx context.Context) error {
				return expectedErr
			},
		},
	}

	success, err := s.executeJobInterface(task, context.Background())

	if success {
		t.Error("Expected success=false")
	}
	if err != expectedErr {
		t.Errorf("Expected error=%v, got: %v", expectedErr, err)
	}
}

// TestExecuteJobInterface_Panic 测试 executeJobInterface panic 恢复
func TestExecuteJobInterface_Panic(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	panicCalled := false
	s.panicHandler = &testPanicHandler{
		handleFunc: func(taskID string, r interface{}, stack []byte) {
			panicCalled = true
		},
	}

	task := &Task{
		ID: "job-task",
		Job: &testJob{
			runFunc: func(ctx context.Context) error {
				panic("job panic")
			},
		},
	}

	success, err := s.executeJobInterface(task, context.Background())

	if success {
		t.Error("Expected success=false after panic")
	}
	if err == nil {
		t.Error("Expected error after panic")
	}
	if !panicCalled {
		t.Error("Expected panic handler to be called")
	}
}

// TestExecuteWithTimeout_ContextCancellation 测试上下文取消
func TestExecuteWithTimeout_ContextCancellation(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	ctx, cancel := context.WithCancel(context.Background())

	// 在任务执行过程中取消上下文
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	success, err := s.executeWithTimeout("test-task", ctx, func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}, nil, s.logger)

	if success {
		t.Error("Expected success=false on cancellation")
	}
	if err == nil {
		t.Error("Expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

// TestExecuteWithTimeout_RaceCondition 测试并发竞争条件
func TestExecuteWithTimeout_RaceCondition(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	var counter int64
	const numGoroutines = 100

	// 并发执行多个任务
	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			success, err := s.executeWithTimeout("test-task", context.Background(), func() error {
				atomic.AddInt64(&counter, 1)
				return nil
			}, nil, s.logger)

			if !success || err != nil {
				t.Errorf("Task execution failed: success=%v, err=%v", success, err)
			}
			done <- true
		}()
	}

	// 等待所有任务完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	if counter != numGoroutines {
		t.Errorf("Expected counter=%d, got: %d", numGoroutines, counter)
	}
}

// TestExecuteWithTimeout_GoroutineCount 测试 goroutine 计数准确性
func TestExecuteWithTimeout_GoroutineCount(t *testing.T) {
	s := newScheduler()
	s.logger = &testLogger{t: t}

	initialGoroutines := runtime.NumGoroutine()

	success, err := s.executeWithTimeout("test-task", context.Background(), func() error {
		return nil
	}, nil, s.logger)

	if !success || err != nil {
		t.Errorf("Task execution failed: success=%v, err=%v", success, err)
	}

	// 等待 goroutine 清理
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// 允许少量差异（最多1个），因为可能有后台 GC 等系统 goroutine
	if abs(finalGoroutines-initialGoroutines) > 1 {
		t.Errorf("Goroutine leak detected: initial=%d, final=%d",
			initialGoroutines, finalGoroutines)
	}
}

// === 辅助类型和函数 ===

// testLogger 测试用日志记录器
type testLogger struct {
	t *testing.T
}

func (l *testLogger) Debugf(format string, args ...interface{}) {
	l.t.Logf("[DEBUG] "+format, args...)
}

func (l *testLogger) Infof(format string, args ...interface{}) {
	l.t.Logf("[INFO] "+format, args...)
}

func (l *testLogger) Warnf(format string, args ...interface{}) {
	l.t.Logf("[WARN] "+format, args...)
}

func (l *testLogger) Errorf(format string, args ...interface{}) {
	l.t.Logf("[ERROR] "+format, args...)
}

// logBuffer 缓冲日志记录器，用于检查日志内容
type logBuffer struct {
	entries []string
}

func (l *logBuffer) Debugf(format string, args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *logBuffer) Infof(format string, args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *logBuffer) Warnf(format string, args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *logBuffer) Errorf(format string, args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *logBuffer) contains(substr string) bool {
	for _, entry := range l.entries {
		if contains(entry, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// testPanicHandler 测试用 panic 处理器
type testPanicHandler struct {
	handleFunc func(taskID string, r interface{}, stack []byte)
}

func (h *testPanicHandler) HandlePanic(taskID string, r interface{}, stack []byte) {
	if h.handleFunc != nil {
		h.handleFunc(taskID, r, stack)
	}
}

// testJob 测试用 Job 实现
type testJob struct {
	runFunc func(ctx context.Context) error
}

func (j *testJob) Name() string {
	return "test-job"
}

func (j *testJob) Run(ctx context.Context) error {
	if j.runFunc != nil {
		return j.runFunc(ctx)
	}
	return nil
}

// abs 返回绝对值
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
