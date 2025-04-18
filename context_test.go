package cron

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 测试支持context的任务结构
type testContextJob struct {
	name      string
	rule      string
	executed  chan bool
	cancelled chan bool
	t         *testing.T
}

func (j *testContextJob) Name() string { return j.name }
func (j *testContextJob) Rule() string { return j.rule }
func (j *testContextJob) Execute() {
	j.t.Logf("Executing standard method for job [%s]", j.name)
	j.executed <- true
}

func (j *testContextJob) ExecuteWithContext(ctx context.Context) {
	j.t.Logf("Executing context-aware method for job [%s]", j.name)

	select {
	case <-time.After(2 * time.Second):
		j.executed <- true
		j.t.Logf("Job [%s] completed normally", j.name)
	case <-ctx.Done():
		j.cancelled <- true
		j.t.Logf("Job [%s] was cancelled: %v", j.name, ctx.Err())
	}
}

// 测试Context接口实现
func TestCrontab_ContextJob(t *testing.T) {
	scheduler := New()

	executed := make(chan bool, 1)
	cancelled := make(chan bool, 1)

	job := &testContextJob{
		name:      "context-job",
		rule:      "*/1 * * * * *",
		executed:  executed,
		cancelled: cancelled,
		t:         t,
	}

	err := scheduler.AddJobInterface(job)
	if err != nil {
		t.Fatal(err)
	}

	scheduler.Start()

	select {
	case <-executed:
		t.Log("Context-aware job executed successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("Context-aware job was not executed")
	}

	scheduler.Stop()
}

// 测试任务超时和Context取消
func TestCrontab_ContextTimeout(t *testing.T) {
	scheduler := New()

	timeoutOccurred := make(chan bool, 1)

	// 使用WithContextFunc创建支持Context的任务
	jobModel, err := NewJobModel("*/1 * * * * *", nil,
		WithContextFunc(func(ctx context.Context) {
			// 启动一个协程检测上下文取消
			go func() {
				<-ctx.Done()
				timeoutOccurred <- true
			}()

			// 睡眠时间超过任务超时时间
			time.Sleep(3 * time.Second)
		}),
		WithTimeout(1*time.Second), // 设置1秒超时
	)
	if err != nil {
		t.Fatal(err)
	}

	err = scheduler.Register("timeout-context-job", jobModel)
	if err != nil {
		t.Fatal(err)
	}

	scheduler.Start()

	// 等待超时发生
	select {
	case <-timeoutOccurred:
		t.Log("Context timeout occurred as expected")
	case <-time.After(5 * time.Second):
		t.Fatal("Context timeout did not occur")
	}

	scheduler.Stop()
}

// 测试改进的并发控制队列
func TestCrontab_ConcurrencyQueue(t *testing.T) {
	scheduler := New()

	maxConcurrent := 2
	executionCount := int32(0)
	startCount := int32(0)
	completedCount := int32(0)

	// 添加互斥锁保护计数
	var mu sync.Mutex
	activeCount := 0
	maxActive := 0

	// 同步完成，确保所有任务都有机会完成
	var wg sync.WaitGroup
	allDone := make(chan struct{})

	// 设置一个执行时间较长的任务，以测试队列行为
	err := scheduler.AddJob(JobConfig{
		Name:          "queue-job",
		Schedule:      "*/1 * * * * *",
		Async:         true,
		MaxConcurrent: maxConcurrent,
	}, func() {
		wg.Add(1)
		defer wg.Done()

		// 增加总执行计数
		atomic.AddInt32(&executionCount, 1)

		// 记录同时活跃的任务数量（使用互斥锁保护）
		mu.Lock()
		activeCount++
		if activeCount > maxActive {
			maxActive = activeCount
		}
		current := atomic.AddInt32(&startCount, 1)
		mu.Unlock()

		t.Logf("Job started, current running: %d", current)

		// 执行时间足够长，以确保多个任务在同一时间进入队列
		time.Sleep(1 * time.Second)

		// 记录完成的任务数
		atomic.AddInt32(&completedCount, 1)

		// 减少活跃计数
		mu.Lock()
		activeCount--
		atomic.AddInt32(&startCount, -1)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}

	// 监听完成信号
	go func() {
		wg.Wait()
		close(allDone)
	}()

	scheduler.Start()

	// 等待足够长的时间，使多个任务有机会排队和执行
	select {
	case <-allDone:
		// 所有任务已完成
	case <-time.After(10 * time.Second):
		// 超时保护
		t.Log("Test timed out waiting for jobs to complete")
	}

	scheduler.Stop()

	// 给所有正在运行的任务时间完成
	time.Sleep(2 * time.Second)

	// 获取最终执行计数
	finalExecution := atomic.LoadInt32(&executionCount)
	finalCompleted := atomic.LoadInt32(&completedCount)

	t.Logf("Total execution attempts: %d, completed: %d, max active: %d",
		finalExecution, finalCompleted, maxActive)

	// 检查最大同时活跃任务数是否遵守并发限制
	if maxActive > maxConcurrent {
		t.Errorf("Concurrency limit violated: maximum should be %d, but %d jobs were running concurrently",
			maxConcurrent, maxActive)
	}

	// 检查是否所有开始的任务都已完成
	if finalExecution != finalCompleted {
		t.Errorf("Not all started jobs completed: started %d, completed %d",
			finalExecution, finalCompleted)
	}
}
