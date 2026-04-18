package cron

import (
	"sync"
	"time"
)

// Stats 任务简化统计信息
type Stats struct {
	ID             string            `json:"id"`              // 任务ID
	Schedule       string            `json:"schedule"`        // 调度表达式
	RunCount       int64             `json:"run_count"`       // 运行次数
	SuccessCount   int64             `json:"success_count"`   // 成功次数
	FailCount      int64             `json:"fail_count"`      // 失败次数
	RetryCount     int64             `json:"retry_count"`     // 重试总次数
	SkippedCount   int64             `json:"skipped_count"`   // 因并发限制被跳过的次数
	TotalDuration  time.Duration     `json:"total_duration"`  // 累计执行时长
	MinDuration    time.Duration     `json:"min_duration"`    // 最小执行时长
	MaxDuration    time.Duration     `json:"max_duration"`    // 最大执行时长
	PeakGoroutines int64             `json:"peak_goroutines"` // 峰值协程数
	Labels         map[string]string `json:"labels"`          // 任务标签
	LastRun        time.Time         `json:"last_run"`        // 最后运行时间
	IsRunning      bool              `json:"is_running"`      // 是否正在运行
	CreatedAt      time.Time         `json:"created_at"`      // 创建时间
	PauseUntil     time.Time         `json:"pause_until"`     // 暂停到期时间
	MisfirePolicy  string            `json:"misfire_policy"`  // Misfire 策略
	HasLastResult  bool              `json:"has_last_result"`
	LastRunSuccess bool              `json:"last_run_success"`
	LastError      string            `json:"last_error"`
}

// Monitor 简化的任务监控器
type Monitor struct {
	stats map[string]*Stats
	mu    sync.RWMutex
}

// newMonitor 创建新的任务监控器
func newMonitor() *Monitor {
	return &Monitor{
		stats: make(map[string]*Stats),
	}
}

// addTask 添加任务到监控
func (m *Monitor) addTask(id, schedule string, createdAt time.Time, labels map[string]string, misfire string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stats[id] = &Stats{
		ID:            id,
		Schedule:      schedule,
		CreatedAt:     createdAt,
		Labels:        cloneLabels(labels),
		MisfirePolicy: misfire,
	}
}

// removeTask 从监控中移除任务
func (m *Monitor) removeTask(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.stats, id)
}

// recordExecution 记录任务执行
func (m *Monitor) recordExecution(id string, duration time.Duration, success bool, retryCount int) {
	m.recordExecutionResult(id, duration, success, retryCount, "")
}

func (m *Monitor) recordExecutionResult(id string, duration time.Duration, success bool, retryCount int, lastError string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, exists := m.stats[id]
	if !exists {
		return
	}

	// 统一使用 mutex 保护，移除冗余的 atomic 操作
	stats.RunCount++
	if success {
		stats.SuccessCount++
	} else {
		stats.FailCount++
	}

	if retryCount > 0 {
		stats.RetryCount += int64(retryCount)
	}

	// 更新执行时长统计
	stats.TotalDuration += duration

	// 更新最小执行时长（首次执行或当前时长更小时）
	if stats.MinDuration == 0 || duration < stats.MinDuration {
		stats.MinDuration = duration
	}

	// 更新最大执行时长
	if duration > stats.MaxDuration {
		stats.MaxDuration = duration
	}

	stats.LastRun = time.Now()
	stats.HasLastResult = true
	stats.LastRunSuccess = success
	stats.LastError = lastError
}

// recordSkip 记录因并发限制被跳过的次数
func (m *Monitor) recordSkip(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stats, exists := m.stats[id]; exists {
		// 统一使用 mutex 保护，移除冗余的 atomic 操作
		stats.SkippedCount++
		stats.LastRun = time.Now()
	}
}

// recordGoroutines 记录协程峰值统计
// 当当前协程数大于已记录的峰值时，更新峰值
func (m *Monitor) recordGoroutines(id string, goroutines int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, exists := m.stats[id]
	if !exists {
		return
	}

	// 统一使用 mutex 保护，简化并发控制逻辑
	if goroutines > stats.PeakGoroutines {
		stats.PeakGoroutines = goroutines
	}
}

// setRunning 设置任务运行状态
func (m *Monitor) setRunning(id string, running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stats, exists := m.stats[id]; exists {
		stats.IsRunning = running
	}
}

// setPauseUntil 记录暂停到期时间
func (m *Monitor) setPauseUntil(id string, until time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stats, exists := m.stats[id]; exists {
		stats.PauseUntil = until
	}
}

// updateSchedule 更新任务的调度表达式
func (m *Monitor) updateSchedule(id, schedule string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stats, exists := m.stats[id]; exists {
		stats.Schedule = schedule
	}
}

// updateTaskMeta 更新任务标签和 Misfire 策略
func (m *Monitor) updateTaskMeta(id string, labels map[string]string, misfire string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stats, exists := m.stats[id]; exists {
		stats.Labels = cloneLabels(labels)
		stats.MisfirePolicy = misfire
	}
}

// GetStats 获取指定任务的统计信息
func (m *Monitor) GetStats(id string) (*Stats, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats, exists := m.stats[id]
	if !exists {
		return nil, false
	}

	statsCopy := *stats
	statsCopy.Labels = cloneLabels(stats.Labels)
	return &statsCopy, true
}

// GetAllStats 获取所有任务的统计信息
func (m *Monitor) GetAllStats() map[string]*Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Stats, len(m.stats))
	for id, stats := range m.stats {
		statsCopy := *stats
		statsCopy.Labels = cloneLabels(stats.Labels)
		result[id] = &statsCopy
	}

	return result
}

// 为Cron添加监控功能
