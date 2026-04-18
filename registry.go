package cron

import (
	"fmt"
	"strings"
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

// NewJobRegistry 创建独立的任务注册表，避免使用全局状态
func NewJobRegistry() *JobRegistry {
	return &JobRegistry{jobs: make(map[string]RegisteredJob)}
}

// SafeRegister 安全注册任务
func (r *JobRegistry) SafeRegister(job RegisteredJob) {
	r.safeRegister(job)
}

// List 返回已注册任务ID
func (r *JobRegistry) List() []string {
	return r.list()
}

// register 注册任务
func (r *JobRegistry) register(job RegisteredJob) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name, err := normalizeTaskID(job.Name())
	if err != nil {
		return fmt.Errorf("job name is invalid: %w", err)
	}
	if _, err := normalizeScheduleSpec(job.Schedule()); err != nil {
		return fmt.Errorf("job schedule is invalid: %w", err)
	}
	if _, exists := r.jobs[name]; exists {
		return fmt.Errorf("job with name %s already registered", name)
	}

	r.jobs[name] = job
	return nil
}

// safeRegister 安全注册
func (r *JobRegistry) safeRegister(job RegisteredJob) {
	if err := r.register(job); err != nil && registryLogger != nil {
		jobName := "<nil>"
		if job != nil {
			jobName = strings.TrimSpace(job.Name())
			if jobName == "" {
				jobName = "<empty>"
			}
		}
		registryLogger.Warnf("Failed to register job %s: %v", jobName, err)
	}
}

// copy 返回注册表副本
func (r *JobRegistry) copy() map[string]RegisteredJob {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]RegisteredJob, len(r.jobs))
	for id, job := range r.jobs {
		result[id] = job
	}
	return result
}

// list 返回任务ID列表
func (r *JobRegistry) list() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.jobs))
	for id := range r.jobs {
		ids = append(ids, id)
	}
	return ids
}

// get 获取单个任务
func (r *JobRegistry) get(id string) (RegisteredJob, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.jobs[id]
	return job, exists
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
	return globalRegistry.register(job)
}

// SetRegistryLogger 设置registry的logger
func SetRegistryLogger(logger Logger) {
	registryLogger = logger
}

// SafeRegisterJob 安全注册任务，永远不会panic
// 适合在init()函数中使用，失败时只记录错误
func SafeRegisterJob(job RegisteredJob) {
	globalRegistry.safeRegister(job)
}

// GetRegisteredJobs 获取所有已注册的任务
func GetRegisteredJobs() map[string]RegisteredJob {
	return globalRegistry.copy()
}

// ScheduleRegistered 将所有已注册的任务添加到调度器
func (c *Cron) ScheduleRegistered(opts ...JobOptions) error {
	jobs := GetRegisteredJobs()
	if len(opts) > 1 {
		return fmt.Errorf("at most one JobOptions value may be provided")
	}

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

// ScheduleFromRegistry 使用指定的注册表调度任务，避免全局状态
func (c *Cron) ScheduleFromRegistry(reg *JobRegistry, opts ...JobOptions) error {
	if reg == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	if len(opts) > 1 {
		return fmt.Errorf("at most one JobOptions value may be provided")
	}

	jobs := reg.copy()

	var defaultOpts JobOptions
	if len(opts) > 0 {
		defaultOpts = opts[0]
	}

	for name, job := range jobs {
		err := c.ScheduleJob(name, job.Schedule(), job, defaultOpts)
		if err != nil {
			return fmt.Errorf("failed to schedule registered job %s: %w", name, err)
		}
	}

	return nil
}

// ListRegistered 列出所有已注册的任务ID
func ListRegistered() []string {
	return globalRegistry.list()
}

// GetRegisteredJob 获取指定ID的已注册任务
func GetRegisteredJob(id string) (RegisteredJob, bool) {
	return globalRegistry.get(id)
}
