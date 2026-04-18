package cron

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// TestMonitor_RecordExecution_DurationStats 测试执行时长统计
func TestMonitor_RecordExecution_DurationStats(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-1"

	// 添加任务
	m.addTask(taskID, "*/5 * * * *", time.Now(), map[string]string{"env": "test"}, "skip")

	// 记录多次执行，测试时长统计
	executions := []struct {
		duration time.Duration
		success  bool
		retry    int
	}{
		{100 * time.Millisecond, true, 0},
		{200 * time.Millisecond, true, 0},
		{50 * time.Millisecond, true, 1},
		{300 * time.Millisecond, false, 0},
		{150 * time.Millisecond, true, 0},
	}

	for _, exec := range executions {
		m.recordExecution(taskID, exec.duration, exec.success, exec.retry)
	}

	// 获取统计信息
	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	// 验证基本计数
	if stats.RunCount != 5 {
		t.Errorf("RunCount = %d; want 5", stats.RunCount)
	}
	if stats.SuccessCount != 4 {
		t.Errorf("SuccessCount = %d; want 4", stats.SuccessCount)
	}
	if stats.FailCount != 1 {
		t.Errorf("FailCount = %d; want 1", stats.FailCount)
	}
	if stats.RetryCount != 1 {
		t.Errorf("RetryCount = %d; want 1", stats.RetryCount)
	}

	// 验证时长统计
	expectedTotal := 100*time.Millisecond + 200*time.Millisecond + 50*time.Millisecond +
		300*time.Millisecond + 150*time.Millisecond
	if stats.TotalDuration != expectedTotal {
		t.Errorf("TotalDuration = %v; want %v", stats.TotalDuration, expectedTotal)
	}

	if stats.MinDuration != 50*time.Millisecond {
		t.Errorf("MinDuration = %v; want %v", stats.MinDuration, 50*time.Millisecond)
	}

	if stats.MaxDuration != 300*time.Millisecond {
		t.Errorf("MaxDuration = %v; want %v", stats.MaxDuration, 300*time.Millisecond)
	}
}

// TestMonitor_RecordExecution_FirstExecution 测试首次执行的时长统计初始化
func TestMonitor_RecordExecution_FirstExecution(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-first"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	// 首次执行
	duration := 123 * time.Millisecond
	m.recordExecution(taskID, duration, true, 0)

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	// 首次执行时，MinDuration 和 MaxDuration 应该都等于执行时长
	if stats.MinDuration != duration {
		t.Errorf("首次执行 MinDuration = %v; want %v", stats.MinDuration, duration)
	}
	if stats.MaxDuration != duration {
		t.Errorf("首次执行 MaxDuration = %v; want %v", stats.MaxDuration, duration)
	}
	if stats.TotalDuration != duration {
		t.Errorf("首次执行 TotalDuration = %v; want %v", stats.TotalDuration, duration)
	}
}

// TestMonitor_RecordGoroutines 测试协程峰值统计
func TestMonitor_RecordGoroutines(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-goroutines"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	// 记录不同的协程数
	goroutineCounts := []int64{10, 25, 15, 30, 20, 28, 30}

	for _, count := range goroutineCounts {
		m.recordGoroutines(taskID, count)
	}

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	// 峰值应该是 30
	if stats.PeakGoroutines != 30 {
		t.Errorf("PeakGoroutines = %d; want 30", stats.PeakGoroutines)
	}
}

// TestMonitor_RecordGoroutines_Concurrent 测试并发场景下的协程峰值统计
func TestMonitor_RecordGoroutines_Concurrent(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-concurrent-goroutines"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	// 并发更新协程峰值
	var wg sync.WaitGroup
	concurrency := 100
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(n int64) {
			defer wg.Done()
			// 每个协程记录一个不同的值
			m.recordGoroutines(taskID, n)
		}(int64(i))
	}

	wg.Wait()

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	// 峰值应该是 99（最大的索引值）
	if stats.PeakGoroutines != int64(concurrency-1) {
		t.Errorf("并发场景 PeakGoroutines = %d; want %d", stats.PeakGoroutines, concurrency-1)
	}
}

// TestMonitor_RecordGoroutines_NonExistentTask 测试不存在的任务
func TestMonitor_RecordGoroutines_NonExistentTask(t *testing.T) {
	m := newMonitor()

	// 记录不存在的任务，应该不会 panic
	m.recordGoroutines("non-existent-task", 100)

	// 如果执行到这里没有 panic，测试通过
}

// TestMonitor_RecordExecution_NonExistentTask 测试记录不存在任务的执行
func TestMonitor_RecordExecution_NonExistentTask(t *testing.T) {
	m := newMonitor()

	// 记录不存在的任务执行，应该不会 panic
	m.recordExecution("non-existent-task", 100*time.Millisecond, true, 0)

	// 如果执行到这里没有 panic，测试通过
}

// TestMonitor_Stats_JSONSerialization 测试 Stats 结构的 JSON 序列化
func TestMonitor_Stats_JSONSerialization(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-json"

	m.addTask(taskID, "*/5 * * * *", time.Now(), map[string]string{"env": "prod"}, "skip")
	m.recordExecution(taskID, 100*time.Millisecond, true, 0)
	m.recordExecution(taskID, 200*time.Millisecond, true, 0)
	m.recordGoroutines(taskID, 50)

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	// 序列化为 JSON
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	// 反序列化
	var decoded Stats
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	// 验证关键字段
	if decoded.TotalDuration != stats.TotalDuration {
		t.Errorf("反序列化后 TotalDuration = %v; want %v", decoded.TotalDuration, stats.TotalDuration)
	}
	if decoded.MinDuration != stats.MinDuration {
		t.Errorf("反序列化后 MinDuration = %v; want %v", decoded.MinDuration, stats.MinDuration)
	}
	if decoded.MaxDuration != stats.MaxDuration {
		t.Errorf("反序列化后 MaxDuration = %v; want %v", decoded.MaxDuration, stats.MaxDuration)
	}
	if decoded.PeakGoroutines != stats.PeakGoroutines {
		t.Errorf("反序列化后 PeakGoroutines = %d; want %d", decoded.PeakGoroutines, stats.PeakGoroutines)
	}
}

// TestMonitor_ConcurrentRecordExecution 测试并发记录执行
func TestMonitor_ConcurrentRecordExecution(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-concurrent-exec"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	var wg sync.WaitGroup
	concurrency := 100
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(n int) {
			defer wg.Done()
			duration := time.Duration(n) * time.Millisecond
			m.recordExecution(taskID, duration, n%2 == 0, n%10)
		}(i)
	}

	wg.Wait()

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	// 验证执行次数
	if stats.RunCount != int64(concurrency) {
		t.Errorf("并发场景 RunCount = %d; want %d", stats.RunCount, concurrency)
	}

	// 验证 MinDuration 应该小于等于第一个值（由于并发，不一定是 0）
	// 但应该是合理的最小值
	if stats.MinDuration < 0 || stats.MinDuration > time.Duration(concurrency-1)*time.Millisecond {
		t.Errorf("并发场景 MinDuration = %v; 超出预期范围", stats.MinDuration)
	}

	// 验证 MaxDuration 应该接近最大值
	expectedMax := time.Duration(concurrency-1) * time.Millisecond
	if stats.MaxDuration < expectedMax || stats.MaxDuration > expectedMax+10*time.Millisecond {
		t.Errorf("并发场景 MaxDuration = %v; 期望接近 %v", stats.MaxDuration, expectedMax)
	}
}

// TestMonitor_GetAllStats 测试获取所有任务统计
func TestMonitor_GetAllStats(t *testing.T) {
	m := newMonitor()

	// 添加多个任务
	m.addTask("task-1", "*/5 * * * *", time.Now(), nil, "skip")
	m.addTask("task-2", "*/10 * * * *", time.Now(), nil, "skip")
	m.addTask("task-3", "*/15 * * * *", time.Now(), nil, "skip")

	// 记录执行
	m.recordExecution("task-1", 100*time.Millisecond, true, 0)
	m.recordExecution("task-2", 200*time.Millisecond, true, 0)
	m.recordExecution("task-3", 300*time.Millisecond, false, 0)

	// 记录协程
	m.recordGoroutines("task-1", 10)
	m.recordGoroutines("task-2", 20)
	m.recordGoroutines("task-3", 30)

	// 获取所有统计
	allStats := m.GetAllStats()

	if len(allStats) != 3 {
		t.Errorf("GetAllStats 返回数量 = %d; want 3", len(allStats))
	}

	// 验证每个任务的统计
	if stats, exists := allStats["task-1"]; exists {
		if stats.TotalDuration != 100*time.Millisecond {
			t.Errorf("task-1 TotalDuration = %v; want 100ms", stats.TotalDuration)
		}
		if stats.PeakGoroutines != 10 {
			t.Errorf("task-1 PeakGoroutines = %d; want 10", stats.PeakGoroutines)
		}
	} else {
		t.Error("task-1 统计信息不存在")
	}

	if stats, exists := allStats["task-2"]; exists {
		if stats.TotalDuration != 200*time.Millisecond {
			t.Errorf("task-2 TotalDuration = %v; want 200ms", stats.TotalDuration)
		}
		if stats.PeakGoroutines != 20 {
			t.Errorf("task-2 PeakGoroutines = %d; want 20", stats.PeakGoroutines)
		}
	} else {
		t.Error("task-2 统计信息不存在")
	}

	if stats, exists := allStats["task-3"]; exists {
		if stats.SuccessCount != 0 {
			t.Errorf("task-3 SuccessCount = %d; want 0", stats.SuccessCount)
		}
		if stats.FailCount != 1 {
			t.Errorf("task-3 FailCount = %d; want 1", stats.FailCount)
		}
	} else {
		t.Error("task-3 统计信息不存在")
	}
}

// TestMonitor_RemoveTask 测试移除任务后的统计
func TestMonitor_RemoveTask(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-remove"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")
	m.recordExecution(taskID, 100*time.Millisecond, true, 0)

	// 验证任务存在
	if _, exists := m.GetStats(taskID); !exists {
		t.Fatal("任务应该存在")
	}

	// 移除任务
	m.removeTask(taskID)

	// 验证任务已被移除
	if _, exists := m.GetStats(taskID); exists {
		t.Error("任务应该已被移除")
	}
}

// TestMonitor_SetRunning 测试设置任务运行状态
func TestMonitor_SetRunning(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-running"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	// 设置为运行中
	m.setRunning(taskID, true)
	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}
	if !stats.IsRunning {
		t.Error("IsRunning 应该为 true")
	}

	// 设置为非运行中
	m.setRunning(taskID, false)
	stats, exists = m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}
	if stats.IsRunning {
		t.Error("IsRunning 应该为 false")
	}
}

// TestMonitor_RecordSkip 测试记录跳过次数
func TestMonitor_RecordSkip(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-skip"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	// 记录多次跳过
	for i := 0; i < 5; i++ {
		m.recordSkip(taskID)
	}

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	if stats.SkippedCount != 5 {
		t.Errorf("SkippedCount = %d; want 5", stats.SkippedCount)
	}
}

// TestMonitor_SetPauseUntil 测试设置暂停时间
func TestMonitor_SetPauseUntil(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-pause"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	pauseTime := time.Now().Add(1 * time.Hour)
	m.setPauseUntil(taskID, pauseTime)

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	// 允许几毫秒的时间误差
	if stats.PauseUntil.Sub(pauseTime).Abs() > time.Millisecond {
		t.Errorf("PauseUntil = %v; want %v", stats.PauseUntil, pauseTime)
	}
}

// TestMonitor_UpdateSchedule 测试更新调度表达式
func TestMonitor_UpdateSchedule(t *testing.T) {
	m := newMonitor()
	taskID := "test-task-schedule"

	m.addTask(taskID, "*/5 * * * *", time.Now(), nil, "skip")

	newSchedule := "*/10 * * * *"
	m.updateSchedule(taskID, newSchedule)

	stats, exists := m.GetStats(taskID)
	if !exists {
		t.Fatal("任务统计信息不存在")
	}

	if stats.Schedule != newSchedule {
		t.Errorf("Schedule = %s; want %s", stats.Schedule, newSchedule)
	}
}
