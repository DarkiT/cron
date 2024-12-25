package cron

import (
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
		t.Fatalf("添加任务失败: %v", err)
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
		t.Fatalf("更新任务失败: %v", err)
	}

	// 测试获取下次执行时间
	nextTime, err := scheduler.NextRuntime("test1")
	if err != nil || nextTime.IsZero() {
		t.Fatalf("获取下次执行时间失败: %v", err)
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
		panic("测试panic")
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduler.Start()

	select {
	case <-panicCaught:
		// panic 被正确捕获
	case <-time.After(2 * time.Second):
		t.Fatal("panic处理器没有捕获到panic")
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
		t.Fatal("任务没有被执行")
	}

	scheduler.Stop()
}

func TestCrontab_AsyncConcurrent(t *testing.T) {
	scheduler := New()
	executionCount := 0

	// 添加一个耗时的异步任务
	err := scheduler.AddJob(JobConfig{
		Name:          "async-concurrent-job",
		Schedule:      "*/1 * * * * *",
		Async:         true,
		MaxConcurrent: 2, // 限制最大并发为2
	}, func() {
		executionCount++
		time.Sleep(2 * time.Second) // 模拟耗时任务
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduler.Start()
	time.Sleep(5 * time.Second)
	scheduler.Stop()

	// 由于最大并发限制为2，执行次数应该小于等于2
	if executionCount > 2 {
		t.Errorf("并发限制失效: 预期最大执行次数为2，实际执行了%d次", executionCount)
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
		t.Fatal("任务没有开始执行")
	}

	// 等待一段时间，让超时生效
	time.Sleep(2 * time.Second)

	// 检查任务是否被中断
	select {
	case <-done:
		// 如果收到完成信号，说明任务没有被正确中断
		t.Error("任务应该因超时而被中断，但实际完成了执行")
	default:
		// 没有收到完成信号，说明任务被正确中断
	}

	scheduler.Stop()
}
