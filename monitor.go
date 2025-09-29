package cron

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stats 任务简化统计信息
type Stats struct {
	ID           string    `json:"id"`            // 任务ID
	Schedule     string    `json:"schedule"`      // 调度表达式
	RunCount     int64     `json:"run_count"`     // 运行次数
	SuccessCount int64     `json:"success_count"` // 成功次数
	LastRun      time.Time `json:"last_run"`      // 最后运行时间
	IsRunning    bool      `json:"is_running"`    // 是否正在运行
	CreatedAt    time.Time `json:"created_at"`    // 创建时间
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
func (m *Monitor) addTask(id, schedule string, createdAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stats[id] = &Stats{
		ID:        id,
		Schedule:  schedule,
		CreatedAt: createdAt,
	}
}

// removeTask 从监控中移除任务
func (m *Monitor) removeTask(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.stats, id)
}

// recordExecution 记录任务执行
func (m *Monitor) recordExecution(id string, duration time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, exists := m.stats[id]
	if !exists {
		return
	}

	atomic.AddInt64(&stats.RunCount, 1)
	if success {
		atomic.AddInt64(&stats.SuccessCount, 1)
	}

	stats.LastRun = time.Now()
}

// setRunning 设置任务运行状态
func (m *Monitor) setRunning(id string, running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stats, exists := m.stats[id]; exists {
		stats.IsRunning = running
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
	return &statsCopy, true
}

// GetAllStats 获取所有任务的统计信息
func (m *Monitor) GetAllStats() map[string]*Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Stats, len(m.stats))
	for id, stats := range m.stats {
		statsCopy := *stats
		result[id] = &statsCopy
	}

	return result
}

// 为Cron添加监控功能
