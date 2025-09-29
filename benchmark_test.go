package cron

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type TestJob struct {
	counter *int64
}

func (j *TestJob) Name() string {
	return "benchmark-test-job"
}

func (j *TestJob) Run(ctx context.Context) error {
	atomic.AddInt64(j.counter, 1)
	return nil
}

// BenchmarkScheduleTasks 测试添加任务的性能
func BenchmarkScheduleTasks(b *testing.B) {
	c := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taskName := fmt.Sprintf("task-%d", i)
		err := c.Schedule(taskName, "*/5 * * * * *", func(ctx context.Context) {})
		if err != nil {
			b.Fatalf("Failed to add task: %v", err)
		}
	}
}

// BenchmarkScheduleJobTasks 测试Job接口任务的性能
func BenchmarkScheduleJobTasks(b *testing.B) {
	c := New()
	counter := int64(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taskName := fmt.Sprintf("job-task-%d", i)
		job := &TestJob{counter: &counter}
		err := c.ScheduleJob(taskName, "*/5 * * * * *", job)
		if err != nil {
			b.Fatalf("Failed to create job task: %v", err)
		}
	}
}

// BenchmarkTaskExecution 测试任务执行性能
func BenchmarkTaskExecution(b *testing.B) {
	c := New()
	counter := int64(0)

	// 添加多个任务
	for i := 0; i < 100; i++ {
		taskName := fmt.Sprintf("exec-task-%d", i)
		err := c.Schedule(taskName, "*/1 * * * * *", func(ctx context.Context) {
			atomic.AddInt64(&counter, 1)
		})
		if err != nil {
			b.Fatalf("Failed to add task: %v", err)
		}
	}

	err := c.Start()
	if err != nil {
		b.Fatalf("Failed to start scheduler: %v", err)
	}
	defer c.Stop()

	b.ResetTimer()

	// 运行基准测试
	startTime := time.Now()
	for time.Since(startTime) < time.Duration(b.N)*time.Millisecond {
		time.Sleep(time.Millisecond)
	}

	b.StopTimer()
	b.Logf("Executed %d tasks in total", atomic.LoadInt64(&counter))
}

// BenchmarkMonitoring 测试监控功能性能
func BenchmarkMonitoring(b *testing.B) {
	c := New()

	// 添加一些任务
	for i := 0; i < 10; i++ {
		taskName := fmt.Sprintf("monitor-task-%d", i)
		err := c.Schedule(taskName, "*/1 * * * * *", func(ctx context.Context) {})
		if err != nil {
			b.Fatalf("Failed to add task: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 测试获取统计信息的性能
		_ = c.GetAllStats()
	}
}

// BenchmarkRegisteredJobs 测试注册任务功能性能
func BenchmarkRegisteredJobs(b *testing.B) {
	// 清理全局注册表
	globalRegistry.mu.Lock()
	globalRegistry.jobs = make(map[string]RegisteredJob)
	globalRegistry.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 注册任务
		job := &RegisteredTestJob{id: fmt.Sprintf("reg-job-%d", i)}
		RegisterJob(job)
	}
}

type RegisteredTestJob struct {
	id      string
	counter *int64
}

func (j *RegisteredTestJob) Name() string {
	return j.id
}

func (j *RegisteredTestJob) Schedule() string {
	return "*/5 * * * * *"
}

func (j *RegisteredTestJob) Run(ctx context.Context) error {
	if j.counter != nil {
		atomic.AddInt64(j.counter, 1)
	}
	return nil
}
