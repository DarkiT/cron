package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestScheduleJobByName 测试新的ScheduleJobByName API
func TestScheduleJobByName(t *testing.T) {
	c := New()
	defer c.Stop()

	// 创建测试任务
	counter := int64(0)
	job := &NamedTestJob{
		name:    "backup-job",
		counter: &counter,
	}

	// 使用新的API - 不需要手动指定名称
	err := c.ScheduleJobByName("*/1 * * * * *", job)
	if err != nil {
		t.Fatalf("ScheduleJobByName failed: %v", err)
	}

	// 启动调度器
	c.Start()
	time.Sleep(2 * time.Second)

	// 验证任务执行
	executed := atomic.LoadInt64(&counter)
	if executed == 0 {
		t.Error("Job should have executed at least once")
	}

	// 验证任务在列表中的名称
	tasks := c.List()
	found := false
	for _, taskID := range tasks {
		if taskID == "backup-job" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Task with name 'backup-job' not found in task list: %v", tasks)
	}

	t.Logf("Job executed %d times with name: %s", executed, job.Name())
}

// TestScheduleJobByNameWithOptions 测试带选项的ScheduleJobByName
func TestScheduleJobByNameWithOptions(t *testing.T) {
	c := New()
	defer c.Stop()

	counter := int64(0)
	job := &NamedTestJob{
		name:    "limited-job",
		counter: &counter,
	}

	// 使用新API并传递选项
	err := c.ScheduleJobByName("*/1 * * * * *", job, JobOptions{
		MaxConcurrent: 1,
		Async:         true,
	})
	if err != nil {
		t.Fatalf("ScheduleJobByName with options failed: %v", err)
	}

	c.Start()
	time.Sleep(2 * time.Second)

	executed := atomic.LoadInt64(&counter)
	if executed == 0 {
		t.Error("Job should have executed at least once")
	}

	t.Logf("Limited job executed %d times", executed)
}

// TestCompareAPIs 比较新旧API的便利性
func TestCompareAPIs(t *testing.T) {
	c := New()
	defer c.Stop()

	counter1 := int64(0)
	counter2 := int64(0)

	job1 := &NamedTestJob{name: "old-api-job", counter: &counter1}
	job2 := &NamedTestJob{name: "new-api-job", counter: &counter2}

	// 旧API - 需要手动指定ID
	err1 := c.ScheduleJob("old-api-job", "*/1 * * * * *", job1)
	if err1 != nil {
		t.Fatalf("Old API failed: %v", err1)
	}

	// 新API - 自动使用Job的Name()
	err2 := c.ScheduleJobByName("*/1 * * * * *", job2)
	if err2 != nil {
		t.Fatalf("New API failed: %v", err2)
	}

	c.Start()
	time.Sleep(2 * time.Second)

	// 两个API都应该正常工作
	executed1 := atomic.LoadInt64(&counter1)
	executed2 := atomic.LoadInt64(&counter2)

	if executed1 == 0 {
		t.Error("Old API job should have executed")
	}
	if executed2 == 0 {
		t.Error("New API job should have executed")
	}

	// 验证任务列表包含两个任务
	tasks := c.List()
	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d: %v", len(tasks), tasks)
	}

	t.Logf("Old API executed %d times, New API executed %d times", executed1, executed2)
}

// NamedTestJob 用于测试的有名称Job
type NamedTestJob struct {
	name    string
	counter *int64
}

func (j *NamedTestJob) Name() string {
	return j.name
}

func (j *NamedTestJob) Run(ctx context.Context) error {
	atomic.AddInt64(j.counter, 1)
	return nil
}

// TestJobFactoryPattern 测试工厂模式的优雅性
func TestJobFactoryPattern(t *testing.T) {
	c := New()
	defer c.Stop()

	// 模拟工厂函数创建Job
	backupJob := NewBackupJob("/data")
	cleanupJob := NewCleanupJob("temp")

	// 使用新API - 更简洁优雅
	err := c.ScheduleJobByName("0 2 * * *", backupJob) // 每天凌晨2点备份
	if err != nil {
		t.Fatalf("Schedule backup job failed: %v", err)
	}

	err = c.ScheduleJobByName("0 3 * * *", cleanupJob) // 每天凌晨3点清理
	if err != nil {
		t.Fatalf("Schedule cleanup job failed: %v", err)
	}

	// 验证任务名称是否正确
	tasks := c.List()
	expectedTasks := []string{"backup-job", "cleanup-job"}

	if len(tasks) != len(expectedTasks) {
		t.Errorf("Expected %d tasks, got %d", len(expectedTasks), len(tasks))
	}

	for _, expected := range expectedTasks {
		found := false
		for _, actual := range tasks {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Task %s not found in list: %v", expected, tasks)
		}
	}

	t.Logf("Factory pattern test passed with tasks: %v", tasks)
}

// 模拟工厂函数
func NewBackupJob(path string) Job {
	return &FactoryTestJob{
		name: "backup-job",
		task: "backup",
		path: path,
	}
}

func NewCleanupJob(target string) Job {
	return &FactoryTestJob{
		name: "cleanup-job",
		task: "cleanup",
		path: target,
	}
}

// FactoryTestJob 工厂模式测试Job
type FactoryTestJob struct {
	name string
	task string
	path string
}

func (j *FactoryTestJob) Name() string {
	return j.name
}

func (j *FactoryTestJob) Run(ctx context.Context) error {
	// 模拟执行任务
	time.Sleep(10 * time.Millisecond)
	return nil
}
