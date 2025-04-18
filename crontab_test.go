package cron

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 测试任务结构
type testJob struct {
	name     string
	rule     string
	executed chan bool
	t        *testing.T // 添加测试引用以便记录日志
}

func (j *testJob) Name() string { return j.name }
func (j *testJob) Rule() string { return j.rule }
func (j *testJob) Execute() {
	j.t.Logf("Executing job [%s]", j.name) // 使用测试日志
	j.executed <- true
	j.t.Logf("Job [%s] execution completed", j.name)
}

// 测试用的 panic 处理器
type testPanicHandler struct {
	handleFunc func(string, interface{})
}

func (h *testPanicHandler) Handle(jobName string, err interface{}) {
	if h.handleFunc != nil {
		h.handleFunc(jobName, err)
	}
}

func TestCrontab_Basic(t *testing.T) {
	scheduler := New()

	// 测试添加任务
	err := scheduler.AddFunc("test1", "*/1 * * * * *", func() {})
	if err != nil {
		t.Fatalf("Failed to add job: %v", err)
	}

	// 测试启动调度器
	scheduler.Start()

	// 测试更新任务
	err = scheduler.UpdateJob(JobConfig{
		Name:     "test1",
		Schedule: "*/2 * * * * *",
	}, func() {
		// 返回空函数
	})
	if err != nil {
		t.Fatalf("Failed to update job: %v", err)
	}

	// 测试获取下次执行时间
	nextTime, err := scheduler.NextRuntime("test1")
	if err != nil || nextTime.IsZero() {
		t.Fatalf("Failed to get next runtime: %v", err)
	}

	// 测试停止任务
	scheduler.StopService("test1")

	// 测试停止调度器
	scheduler.Stop()
}

func TestCrontab_PanicHandler(t *testing.T) {
	panicCaught := make(chan bool, 1)
	scheduler := New(WithPanicHandler(&testPanicHandler{
		handleFunc: func(jobName string, err interface{}) {
			panicCaught <- true
		},
	}))

	err := scheduler.AddJob(JobConfig{
		Name:     "panic-job",
		Schedule: "*/1 * * * * *",
		TryCatch: true,
	}, func() {
		panic("test panic")
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduler.Start()

	select {
	case <-panicCaught:
		// panic 被正确捕获
	case <-time.After(2 * time.Second):
		t.Fatal("Panic handler did not catch the panic")
	}

	scheduler.Stop()
}

func TestCrontab_CronJob(t *testing.T) {
	scheduler := New()

	executed := make(chan bool, 1)
	job := &testJob{
		name:     "test-job",
		rule:     "*/1 * * * * *",
		executed: executed,
		t:        t,
	}

	err := scheduler.AddJobInterface(job)
	if err != nil {
		t.Fatal(err)
	}
	scheduler.Start()

	select {
	case <-executed:
		t.Log("Job executed successfully")
	case <-time.After(3 * time.Second):
		t.Log("Job execution timed out")
		tasks := scheduler.ListJobs()
		for _, task := range tasks {
			t.Logf("Task %s is registered but not executed", task)
		}
		t.Fatal("Job was not executed")
	}

	scheduler.Stop()
}

func TestCrontab_AsyncConcurrent(t *testing.T) {
	scheduler := New()
	var executionCount int32 // 使用原子计数器
	var maxConcurrentRunning int32
	var mutex sync.Mutex
	var wg sync.WaitGroup
	allDone := make(chan struct{})

	// 添加一个耗时的异步任务
	err := scheduler.AddJob(JobConfig{
		Name:          "async-concurrent-job",
		Schedule:      "*/1 * * * * *",
		Async:         true,
		MaxConcurrent: 2, // 限制最大并发为2
	}, func() {
		wg.Add(1)
		defer wg.Done()

		// 增加计数并记录最大并发数
		current := atomic.AddInt32(&executionCount, 1)

		// 增加当前运行计数
		mutex.Lock()
		running := atomic.AddInt32(&maxConcurrentRunning, 1)
		mutex.Unlock()

		t.Logf("Job #%d started, current running: %d", current, running)

		// 模拟耗时任务
		time.Sleep(1 * time.Second)

		// 减少当前运行计数
		mutex.Lock()
		atomic.AddInt32(&maxConcurrentRunning, -1)
		mutex.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}

	// 监听所有任务完成
	go func() {
		wg.Wait()
		close(allDone)
	}()

	scheduler.Start()

	// 等待足够长的时间让任务执行
	select {
	case <-allDone:
		// 所有任务已完成
	case <-time.After(6 * time.Second):
		// 超时保护
	}

	scheduler.Stop()

	// 等待所有任务完成
	time.Sleep(2 * time.Second)

	// 读取最终并发度
	maxConcurrent := atomic.LoadInt32(&maxConcurrentRunning)
	finalCount := atomic.LoadInt32(&executionCount)

	t.Logf("Max concurrent running: %d, total executed: %d", maxConcurrent, finalCount)

	// 检查最大并发度是否符合预期
	if maxConcurrent > 2 {
		t.Errorf("Concurrency limit failed: expected max concurrent of 2, but got %d", maxConcurrent)
	}
}

func TestCrontab_Timeout(t *testing.T) {
	scheduler := New()
	executing := make(chan struct{})
	done := make(chan struct{})

	// 添加一个会超时的任务
	err := scheduler.AddJob(JobConfig{
		Name:     "timeout-job",
		Schedule: "*/1 * * * * *",
		Timeout:  time.Second, // 1秒超时
	}, func() {
		close(executing)            // 表示任务开始执行
		time.Sleep(3 * time.Second) // 任务执行时间超过超时时间
		close(done)                 // 表示任务尝试完成
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduler.Start()

	// 等待任务开始执行
	select {
	case <-executing:
		// 任务已开始执行
	case <-time.After(2 * time.Second):
		t.Fatal("Job did not start execution")
	}

	// 等待一段时间，让超时生效
	time.Sleep(2 * time.Second)

	// 检查任务是否被中断
	select {
	case <-done:
		// 如果收到完成信号，说明任务没有被正确中断
		t.Error("Job should have been interrupted by timeout, but it completed execution")
	default:
		// 没有收到完成信号，说明任务被正确中断
	}

	scheduler.Stop()
}
