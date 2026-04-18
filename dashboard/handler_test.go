package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkit/cron"
	"github.com/darkit/cron/history"
)

// setupTestCron 创建测试用的 cron 调度器
func setupTestCron(t *testing.T) *cron.Cron {
	tmpDir := t.TempDir()

	// 创建历史记录存储
	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	recorder := history.NewHistoryRecorder(storage)
	t.Cleanup(func() {
		recorder.Close()
		storage.Close()
	})

	// 创建调度器
	c := cron.New(cron.WithHistoryRecorder(recorder))

	// 添加测试任务
	c.Schedule("test-task-1", "@every 1h", func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	})

	c.Schedule("test-task-2", "@every 30m", func(ctx context.Context) {
		time.Sleep(20 * time.Millisecond)
	})

	c.Start()
	t.Cleanup(func() {
		c.Stop()
	})

	return c
}

// TestGetTasks 测试获取任务列表
func TestGetTasks(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	req := httptest.NewRequest("GET", "/api/tasks", nil)
	w := httptest.NewRecorder()

	handler.GetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var tasks []TaskInfo
	if err := json.NewDecoder(w.Body).Decode(&tasks); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}

	// 验证任务信息
	taskIDs := make(map[string]bool)
	for _, task := range tasks {
		taskIDs[task.ID] = true
		if task.NextRun.IsZero() {
			t.Errorf("Task %s has zero NextRun time", task.ID)
		}
	}

	if !taskIDs["test-task-1"] || !taskIDs["test-task-2"] {
		t.Error("Expected tasks not found in response")
	}
}

// TestGetTask 测试获取单个任务详情
func TestGetTask(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	// 设置路由
	req := httptest.NewRequest("GET", "/api/tasks/test-task-1", nil)
	w := httptest.NewRecorder()
	handler.GetTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var task TaskInfo
	if err := json.NewDecoder(w.Body).Decode(&task); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if task.ID != "test-task-1" {
		t.Errorf("Expected task ID 'test-task-1', got '%s'", task.ID)
	}

	if task.NextRun.IsZero() {
		t.Error("Task has zero NextRun time")
	}
}

// TestGetTaskNotFound 测试获取不存在的任务
func TestGetTaskNotFound(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	req := httptest.NewRequest("GET", "/api/tasks/non-existent", nil)
	w := httptest.NewRecorder()
	handler.GetTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestGetStats 测试获取统计信息
func TestGetStats(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()

	handler.GetStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var stats StatsInfo
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if stats.TotalTasks != 2 {
		t.Errorf("Expected 2 total tasks, got %d", stats.TotalTasks)
	}

	// 成功率应该在 0-100 之间
	if stats.SuccessRate < 0 || stats.SuccessRate > 100 {
		t.Errorf("Invalid success rate: %f", stats.SuccessRate)
	}
}

func TestGetStatsUsesMonitorDurationsWithoutHistoryScanRequirement(t *testing.T) {
	c := cron.New()
	handler := NewHandler(c)

	err := c.Schedule("fast-task", "*/1 * * * * *", func(ctx context.Context) {
		time.Sleep(15 * time.Millisecond)
	})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer c.Stop()

	time.Sleep(1200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	handler.GetStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var stats StatsInfo
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if stats.TotalRuns == 0 {
		t.Fatal("expected monitor to observe at least one run")
	}
	if stats.TotalDuration == "0s" {
		t.Fatalf("expected total duration from monitor, got %q", stats.TotalDuration)
	}
	if stats.AvgDuration == "0s" {
		t.Fatalf("expected avg duration from monitor, got %q", stats.AvgDuration)
	}
	if stats.HistoryRecords != 0 {
		t.Fatalf("expected no persisted history records without recorder, got %d", stats.HistoryRecords)
	}
}

func TestGetTaskReflectsLatestFailureStatus(t *testing.T) {
	c := cron.New()
	handler := NewHandler(c)

	var runs int
	err := c.Schedule("status-task", "*/1 * * * * *", func(ctx context.Context) {
		runs++
		if runs >= 2 {
			panic(errors.New("boom on second run"))
		}
	})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer c.Stop()

	time.Sleep(2200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/tasks/status-task", nil)
	w := httptest.NewRecorder()
	handler.GetTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var task TaskInfo
	if err := json.NewDecoder(w.Body).Decode(&task); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if task.LastRunStatus != "failed" {
		t.Fatalf("expected latest run status failed, got %q", task.LastRunStatus)
	}
	if !strings.Contains(task.LastError, "boom on second run") {
		t.Fatalf("expected last error to be propagated, got %q", task.LastError)
	}
}

func TestUnfuseTaskIsDeprecatedResumeAlias(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	if err := c.Pause("test-task-1"); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/test-task-1/unfuse", nil)
	w := httptest.NewRecorder()
	handler.UnfuseTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Message, "deprecated alias") {
		t.Fatalf("expected deprecated alias message, got %q", resp.Message)
	}

	task, ok := c.GetTask("test-task-1")
	if !ok {
		t.Fatal("expected task to exist")
	}
	if task.IsPaused {
		t.Fatal("expected unfuse alias to resume paused task")
	}
}

// TestGetHistory 测试获取历史记录
func TestGetHistory(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	// 等待一些任务执行并生成历史记录
	time.Sleep(200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/history?limit=10", nil)
	w := httptest.NewRecorder()

	handler.GetHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response HistoryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.PageSize != 10 {
		t.Errorf("Expected page size 10, got %d", response.PageSize)
	}
}

// TestGetHistoryWithFilter 测试带过滤条件的历史查询
func TestGetHistoryWithFilter(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	// 等待一些任务执行
	time.Sleep(200 * time.Millisecond)

	// 测试按任务ID过滤
	req := httptest.NewRequest("GET", "/api/history?taskId=test-task-1&limit=5", nil)
	w := httptest.NewRecorder()

	handler.GetHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response HistoryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 验证所有记录都是指定的任务
	for _, record := range response.Records {
		if record.TaskID != "test-task-1" {
			t.Errorf("Expected all records to be for task 'test-task-1', got '%s'", record.TaskID)
		}
	}
}

// TestRemoveTask 测试移除任务
func TestRemoveTask(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	// 移除任务
	req := httptest.NewRequest("DELETE", "/api/tasks/test-task-1", nil)
	w := httptest.NewRecorder()
	handler.RemoveTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// 验证任务已被移除
	tasks := c.List()
	for _, taskID := range tasks {
		if taskID == "test-task-1" {
			t.Error("Task should have been removed")
		}
	}
}

// TestRemoveNonExistentTask 测试移除不存在的任务
func TestRemoveNonExistentTask(t *testing.T) {
	c := setupTestCron(t)
	handler := NewHandler(c)

	req := httptest.NewRequest("DELETE", "/api/tasks/non-existent", nil)
	w := httptest.NewRecorder()
	handler.RemoveTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}
