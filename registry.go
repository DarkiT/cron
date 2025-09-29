package cron

import (
	"fmt"
	"sync"
)

// RegisteredJob 可注册的任务接口
type RegisteredJob interface {
	Job
	Name() string     // 返回任务唯一标识
	Schedule() string // 返回cron调度表达式
}

// JobRegistry 全局任务注册表
type JobRegistry struct {
	jobs map[string]RegisteredJob
	mu   sync.RWMutex
}

var (
	globalRegistry = &JobRegistry{
		jobs: make(map[string]RegisteredJob),
	}

	// 全局logger实例，用于registry日志记录
	registryLogger Logger = NewDefaultLogger()
)

// RegisterJob 注册一个任务到全局注册表
func RegisterJob(job RegisteredJob) error {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	// 🌟 统一使用Name()作为注册key，保持与Job接口一致
	name := job.Name()

	if _, exists := globalRegistry.jobs[name]; exists {
		return fmt.Errorf("job with name %s already registered", name)
	}

	globalRegistry.jobs[name] = job
	return nil
}

// SetRegistryLogger 设置registry的logger
func SetRegistryLogger(logger Logger) {
	registryLogger = logger
}

// SafeRegisterJob 安全注册任务，永远不会panic
// 适合在init()函数中使用，失败时只记录错误
func SafeRegisterJob(job RegisteredJob) {
	if err := RegisterJob(job); err != nil {
		if registryLogger != nil {
			registryLogger.Warnf("Failed to register job %s: %v", job.Name(), err)
		}
	}
}

// GetRegisteredJobs 获取所有已注册的任务
func GetRegisteredJobs() map[string]RegisteredJob {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	result := make(map[string]RegisteredJob, len(globalRegistry.jobs))
	for id, job := range globalRegistry.jobs {
		result[id] = job
	}
	return result
}

// ScheduleRegistered 将所有已注册的任务添加到调度器
func (c *Cron) ScheduleRegistered(opts ...JobOptions) error {
	jobs := GetRegisteredJobs()

	var defaultOpts JobOptions
	if len(opts) > 0 {
		defaultOpts = opts[0]
	}

	for name, job := range jobs {
		// 使用注册时的name作为任务标识，与Job.Name()保持一致
		err := c.ScheduleJob(name, job.Schedule(), job, defaultOpts)
		if err != nil {
			return fmt.Errorf("failed to schedule registered job %s: %w", name, err)
		}
	}

	return nil
}

// ListRegistered 列出所有已注册的任务ID
func ListRegistered() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	ids := make([]string, 0, len(globalRegistry.jobs))
	for id := range globalRegistry.jobs {
		ids = append(ids, id)
	}
	return ids
}

// GetRegisteredJob 获取指定ID的已注册任务
func GetRegisteredJob(id string) (RegisteredJob, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	job, exists := globalRegistry.jobs[id]
	return job, exists
}
