package cron

import (
	"context"
	"slices"
	"testing"
)

// TestNameUnification 测试Name()方法统一化
func TestNameUnification(t *testing.T) {
	// 清理全局注册表
	globalRegistry.jobs = make(map[string]RegisteredJob)

	// 测试基本注册功能
	job := &RegistryTestJob{}
	err := RegisterJob(job)
	if err != nil {
		t.Fatalf("Register job failed: %v", err)
	}

	// 验证使用Name()作为key
	jobs := GetRegisteredJobs()
	if _, exists := jobs["test-job"]; !exists {
		t.Error("Job should be registered with Name() as key")
	}

	// 测试ScheduleRegistered使用Name()
	c := New()
	defer c.Stop()

	err = c.ScheduleRegistered()
	if err != nil {
		t.Fatalf("ScheduleRegistered failed: %v", err)
	}

	// 验证任务使用Name()注册
	tasks := c.List()
	found := slices.Contains(tasks, "test-job")
	if !found {
		t.Errorf("Task should be scheduled with Name() as ID, tasks: %v", tasks)
	}

	// 清理
	globalRegistry.jobs = make(map[string]RegisteredJob)
}

// RegistryTestJob 简化的测试任务
type RegistryTestJob struct{}

func (j *RegistryTestJob) Name() string                  { return "test-job" }
func (j *RegistryTestJob) Schedule() string              { return "*/5 * * * * *" }
func (j *RegistryTestJob) Run(ctx context.Context) error { return nil }

// ConsistentTestJob 保持向后兼容的测试任务
type ConsistentTestJob struct{}

func (j *ConsistentTestJob) Name() string                  { return "consistent-job" }
func (j *ConsistentTestJob) Schedule() string              { return "*/5 * * * * *" }
func (j *ConsistentTestJob) Run(ctx context.Context) error { return nil }

// TestAPIRecommendations 测试API推荐使用方式
func TestAPIRecommendations(t *testing.T) {
	c := New()
	defer c.Stop()

	// 🥇 首选：ScheduleJobByName
	backupJob := &ConsistentTestJob{}
	err := c.ScheduleJobByName("*/10 * * * * *", backupJob)
	if err != nil {
		t.Fatalf("ScheduleJobByName failed: %v", err)
	}

	// 🥈 备选：ScheduleJob 显式指定名称
	err = c.ScheduleJob("custom-backup", "*/10 * * * * *", backupJob)
	if err != nil {
		t.Fatalf("ScheduleJob failed: %v", err)
	}

	// 验证两个任务都被正确添加
	tasks := c.List()
	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d: %v", len(tasks), tasks)
	}

	expectedTasks := []string{"consistent-job", "custom-backup"}
	for _, expected := range expectedTasks {
		found := slices.Contains(tasks, expected)
		if !found {
			t.Errorf("Task %s not found in %v", expected, tasks)
		}
	}

	t.Logf("✅ API推荐测试通过，任务列表: %v", tasks)
}
