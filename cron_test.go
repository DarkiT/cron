package cron

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestJob 测试用的Job实现
type TestJobImpl struct {
	counter *int64
	name    string
}

func (j *TestJobImpl) Name() string {
	if j.name != "" {
		return j.name
	}
	return "test-job"
}

func (j *TestJobImpl) Run(ctx context.Context) error {
	atomic.AddInt64(j.counter, 1)
	return nil
}

// TestCron_Basic 测试基本功能
func TestCron_Basic(t *testing.T) {
	c := New()
	counter := int64(0)

	err := c.Schedule("test", "*/1 * * * * *", func(ctx context.Context) {
		atomic.AddInt64(&counter, 1)
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(2 * time.Second)
	c.Stop()

	if counter == 0 {
		t.Errorf("Expected counter > 0, got %d", counter)
	}
}

// TestCron_JobInterface 测试Job接口功能
func TestCron_JobInterface(t *testing.T) {
	c := New()
	counter := int64(0)
	job := &TestJobImpl{counter: &counter, name: "test-job"}

	err := c.ScheduleJob("test-job", "*/1 * * * * *", job, JobOptions{
		Async: true,
	})
	if err != nil {
		t.Fatalf("Failed to schedule job: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(2 * time.Second)
	c.Stop()

	if counter == 0 {
		t.Errorf("Expected counter > 0, got %d", counter)
	}
}

// TestCron_WithTimeout 测试超时功能
func TestCron_WithTimeout(t *testing.T) {
	c := New()
	counter := int64(0)
	job := &TestJobImpl{counter: &counter, name: "timeout-job"}

	err := c.ScheduleJob("timeout-job", "*/1 * * * * *", job, JobOptions{
		Timeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to schedule job with timeout: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(2 * time.Second)
	c.Stop()

	if counter == 0 {
		t.Errorf("Expected counter > 0, got %d", counter)
	}
}

// TestCron_Context 测试上下文功能
func TestCron_Context(t *testing.T) {
	c := New()
	var receivedCtx context.Context

	err := c.Schedule("context-test", "*/1 * * * * *", func(ctx context.Context) {
		receivedCtx = ctx
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)
	c.Stop()

	if receivedCtx == nil {
		t.Errorf("Expected context to be passed to task")
	}
}

// TestCron_Remove 测试移除任务
func TestCron_Remove(t *testing.T) {
	c := New()
	counter := int64(0)

	err := c.Schedule("removable", "*/1 * * * * *", func(ctx context.Context) {
		atomic.AddInt64(&counter, 1)
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	// 启动后立即移除
	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	err = c.Remove("removable")
	if err != nil {
		t.Fatalf("Failed to remove task: %v", err)
	}

	time.Sleep(2 * time.Second)
	c.Stop()

	// 移除后不应该继续增长太多
	if counter > 5 {
		t.Errorf("Expected counter <= 5 after removal, got %d", counter)
	}
}

// TestCron_List 测试列出任务
func TestCron_List(t *testing.T) {
	c := New()

	tasks := []string{"task1", "task2", "task3"}
	for _, taskName := range tasks {
		err := c.Schedule(taskName, "*/5 * * * * *", func(ctx context.Context) {})
		if err != nil {
			t.Fatalf("Failed to schedule task %s: %v", taskName, err)
		}
	}

	list := c.List()
	if len(list) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(list))
	}

	// 检查是否包含所有任务
	for _, expected := range tasks {
		found := false
		for _, actual := range list {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Task %s not found in list", expected)
		}
	}
}

// TestCron_NextRun 测试获取下次运行时间
func TestCron_NextRun(t *testing.T) {
	c := New()

	err := c.Schedule("next-run-test", "*/5 * * * * *", func(ctx context.Context) {})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	nextRun, err := c.NextRun("next-run-test")
	if err != nil {
		t.Fatalf("Failed to get next run time: %v", err)
	}

	if nextRun.IsZero() {
		t.Errorf("Expected valid next run time")
	}

	// 测试不存在的任务
	_, err = c.NextRun("non-existent")
	if err == nil {
		t.Errorf("Expected error for non-existent task")
	}
}

// TestCron_Stats 测试统计功能
func TestCron_Stats(t *testing.T) {
	c := New()
	counter := int64(0)

	err := c.Schedule("stats-test", "*/1 * * * * *", func(ctx context.Context) {
		atomic.AddInt64(&counter, 1)
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(2 * time.Second)
	c.Stop()

	stats, exists := c.GetStats("stats-test")
	if !exists {
		t.Fatalf("Expected stats to exist for stats-test")
	}

	if stats.RunCount == 0 {
		t.Errorf("Expected RunCount > 0, got %d", stats.RunCount)
	}

	allStats := c.GetAllStats()
	if len(allStats) == 0 {
		t.Errorf("Expected at least 1 task in all stats")
	}
}

// TestRegisteredJobs 测试注册任务功能
func TestRegisteredJobs(t *testing.T) {
	// 清理全局注册表
	globalRegistry.mu.Lock()
	globalRegistry.jobs = make(map[string]RegisteredJob)
	globalRegistry.mu.Unlock()

	// 创建测试任务
	testJob := &TestRegisteredJob{
		name:     "registered-test",
		schedule: "*/2 * * * * *",
		counter:  new(int64),
	}

	// 注册任务
	RegisterJob(testJob)

	// 检查任务是否被注册
	registered := ListRegistered()
	if len(registered) != 1 || registered[0] != "registered-test" {
		t.Errorf("Expected registered job 'registered-test', got %v", registered)
	}

	// 创建调度器并调度已注册的任务
	c := New()
	err := c.ScheduleRegistered()
	if err != nil {
		t.Fatalf("Failed to schedule registered jobs: %v", err)
	}

	// 启动并运行一段时间
	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(3 * time.Second)
	c.Stop()

	// 检查任务是否执行
	if atomic.LoadInt64(testJob.counter) == 0 {
		t.Errorf("Expected registered job to execute at least once")
	}
}

// TestRegisteredJob 测试用的注册任务
type TestRegisteredJob struct {
	name     string
	schedule string
	counter  *int64
}

func (j *TestRegisteredJob) Name() string {
	return j.name
}

func (j *TestRegisteredJob) Schedule() string {
	return j.schedule
}

func (j *TestRegisteredJob) Run(ctx context.Context) error {
	atomic.AddInt64(j.counter, 1)
	return nil
}

// TestConcurrency 测试并发安全性
func TestConcurrency(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	counter := int64(0)

	// 并发添加任务
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			taskName := fmt.Sprintf("concurrent-task-%d", id)
			err := c.Schedule(taskName, "*/1 * * * * *", func(ctx context.Context) {
				atomic.AddInt64(&counter, 1)
			})
			if err != nil {
				t.Errorf("Failed to schedule task %s: %v", taskName, err)
			}
		}(i)
	}

	wg.Wait()

	// 检查任务数量
	tasks := c.List()
	if len(tasks) != 10 {
		t.Errorf("Expected 10 tasks, got %d", len(tasks))
	}

	// 运行测试
	err := c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(2 * time.Second)
	c.Stop()

	if counter == 0 {
		t.Errorf("Expected counter > 0, got %d", counter)
	}
}
