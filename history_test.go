package cron

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/darkit/cron/history"
)

func cleanupHistoryStorage(t *testing.T, storage history.Storage) {
	t.Helper()
	if storage == nil {
		return
	}
	if err := storage.Close(); err != nil {
		t.Fatalf("Failed to close storage: %v", err)
	}
}

func cleanupHistoryRecorder(t *testing.T, recorder history.Recorder) {
	t.Helper()
	if recorder == nil {
		return
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to close recorder: %v", err)
	}
}

func cleanupCronHistoryStorage(t *testing.T, storage *history.FileStorage) {
	t.Helper()
	if storage == nil {
		return
	}
	if err := storage.Close(); err != nil {
		t.Fatalf("Failed to close storage: %v", err)
	}
}

// TestHistoryBasic 测试基本的历史记录功能
func TestHistoryBasic(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	// 创建存储
	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupHistoryStorage(t, storage)

	// 创建记录器
	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer cleanupHistoryRecorder(t, recorder)

	// 创建调度器
	c := New(WithHistoryRecorder(recorder))

	// 添加任务
	var executed atomic.Bool
	err = c.Schedule("test-task", "@every 1s", func(ctx context.Context) {
		executed.Store(true)
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer c.Stop()

	// 等待任务执行
	time.Sleep(1500 * time.Millisecond)

	// 验证任务执行
	if !executed.Load() {
		t.Error("Task was not executed")
	}

	// 等待记录写入
	time.Sleep(200 * time.Millisecond)

	// 查询历史记录
	records, err := c.QueryHistory(history.RecordFilter{
		TaskID: "test-task",
	})
	if err != nil {
		t.Fatalf("Failed to query history: %v", err)
	}

	if len(records) == 0 {
		t.Error("Expected at least 1 history record")
	}

	// 验证记录内容
	record := records[0]
	if record.TaskID != "test-task" {
		t.Errorf("Expected TaskID=test-task, got %s", record.TaskID)
	}
	if !record.Success {
		t.Error("Expected Success=true")
	}
	if record.RetryCount != 0 {
		t.Errorf("Expected RetryCount=0, got %d", record.RetryCount)
	}
}

// TestHistoryWithRetry 测试重试机制与历史记录的集成
func TestHistoryWithRetry(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupHistoryStorage(t, storage)

	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer cleanupHistoryRecorder(t, recorder)

	c := New(WithHistoryRecorder(recorder))

	// 创建会失败2次然后成功的任务
	attempts := 0
	err = c.Schedule("retry-task", "@every 1s", func(ctx context.Context) {
		attempts++
		if attempts < 3 {
			panic("test failure")
		}
	}, JobOptions{
		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
		MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// 等待任务执行和重试完成（第一次执行会重试2次然后成功）
	// 100ms 重试间隔 * 2 次重试 + 一些余量 = 至少 500ms
	time.Sleep(800 * time.Millisecond)

	c.Stop()

	// 等待记录写入
	time.Sleep(300 * time.Millisecond)

	// 验证任务至少执行了3次（2次失败 + 1次成功）
	if attempts < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", attempts)
	}

	// 查询历史记录
	records, err := c.QueryHistory(history.RecordFilter{
		TaskID: "retry-task",
	})
	if err != nil {
		t.Fatalf("Failed to query history: %v", err)
	}

	if len(records) == 0 {
		t.Fatal("Expected at least 1 history record")
	}

	// 找到成功的记录
	var successRecord *history.ExecutionRecord
	for _, record := range records {
		if record.Success {
			successRecord = record
			break
		}
	}

	if successRecord == nil {
		t.Fatal("Expected to find a successful record")
	}

	// 验证重试次数
	if successRecord.RetryCount != 2 {
		t.Errorf("Expected RetryCount=2, got %d", successRecord.RetryCount)
	}
}

// TestHistoryQuery 测试历史记录查询功能
func TestHistoryQuery(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupHistoryStorage(t, storage)

	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer cleanupHistoryRecorder(t, recorder)

	c := New(WithHistoryRecorder(recorder))

	// 添加两个任务
	err = c.Schedule("task1", "@every 500ms", func(ctx context.Context) {
		// 成功任务
	})
	if err != nil {
		t.Fatalf("Failed to schedule task1: %v", err)
	}

	err = c.Schedule("task2", "@every 500ms", func(ctx context.Context) {
		panic("always fail")
	}, JobOptions{
		MaxRetries: 0,
	})
	if err != nil {
		t.Fatalf("Failed to schedule task2: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer c.Stop()

	// 等待任务执行
	time.Sleep(1500 * time.Millisecond)

	// 等待记录写入
	time.Sleep(200 * time.Millisecond)

	// 测试按任务ID查询
	records1, err := c.QueryHistory(history.RecordFilter{
		TaskID: "task1",
	})
	if err != nil {
		t.Fatalf("Failed to query task1: %v", err)
	}
	if len(records1) == 0 {
		t.Error("Expected history records for task1")
	}

	// 测试查询所有任务
	allRecords, err := c.QueryHistory(history.RecordFilter{})
	if err != nil {
		t.Fatalf("Failed to query all records: %v", err)
	}
	if len(allRecords) < 2 {
		t.Errorf("Expected at least 2 records, got %d", len(allRecords))
	}

	// 测试仅查询成功的记录
	successRecords, err := c.QueryHistory(history.RecordFilter{
		SuccessOnly: true,
	})
	if err != nil {
		t.Fatalf("Failed to query success records: %v", err)
	}
	for _, record := range successRecords {
		if !record.Success {
			t.Error("Found failed record in success-only query")
		}
	}

	// 测试仅查询失败的记录
	failedRecords, err := c.QueryHistory(history.RecordFilter{
		FailedOnly: true,
	})
	if err != nil {
		t.Fatalf("Failed to query failed records: %v", err)
	}
	for _, record := range failedRecords {
		if record.Success {
			t.Error("Found success record in failed-only query")
		}
	}
}

// TestHistoryCount 测试历史记录统计功能
func TestHistoryCount(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupHistoryStorage(t, storage)

	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer cleanupHistoryRecorder(t, recorder)

	c := New(WithHistoryRecorder(recorder))

	err = c.Schedule("count-task", "@every 300ms", func(ctx context.Context) {
		// 任务执行
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer c.Stop()

	// 等待多次执行
	time.Sleep(1500 * time.Millisecond)

	// 等待记录写入
	time.Sleep(200 * time.Millisecond)

	// 统计记录数量
	count, err := c.CountHistory(history.RecordFilter{
		TaskID: "count-task",
	})
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if count < 3 {
		t.Errorf("Expected at least 3 records, got %d", count)
	}
}

func TestHistoryRecorderContinuesAfterRestart(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupHistoryStorage(t, storage)

	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer cleanupHistoryRecorder(t, recorder)

	c := New(WithHistoryRecorder(recorder))

	err = c.Schedule("restart-history-task", "@every 80ms", func(ctx context.Context) {})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	time.Sleep(180 * time.Millisecond)
	c.Stop()
	time.Sleep(150 * time.Millisecond)

	firstCount, err := c.CountHistory(history.RecordFilter{TaskID: "restart-history-task"})
	if err != nil {
		t.Fatalf("Failed to count history after first stop: %v", err)
	}
	if firstCount == 0 {
		t.Fatal("expected history records after first run")
	}

	if err := c.Start(); err != nil {
		t.Fatalf("second start failed: %v", err)
	}
	time.Sleep(180 * time.Millisecond)
	c.Stop()
	time.Sleep(150 * time.Millisecond)

	secondCount, err := c.CountHistory(history.RecordFilter{TaskID: "restart-history-task"})
	if err != nil {
		t.Fatalf("Failed to count history after restart: %v", err)
	}
	if secondCount <= firstCount {
		t.Fatalf("expected history to continue recording after restart: first=%d second=%d", firstCount, secondCount)
	}
}

// TestHistoryCleanup 测试历史记录清理功能
func TestHistoryCleanup(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupHistoryStorage(t, storage)

	// 手动插入一些旧记录
	oldTime := time.Now().Add(-48 * time.Hour)
	oldRecord := &history.ExecutionRecord{
		ID:        "old-task_123",
		TaskID:    "old-task",
		StartTime: oldTime,
		EndTime:   oldTime.Add(1 * time.Second),
		Duration:  1 * time.Second,
		Success:   true,
	}
	err = storage.Save(oldRecord)
	if err != nil {
		t.Fatalf("Failed to save old record: %v", err)
	}

	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer cleanupHistoryRecorder(t, recorder)

	c := New(WithHistoryRecorder(recorder))

	// 添加新任务
	err = c.Schedule("new-task", "@every 500ms", func(ctx context.Context) {})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	time.Sleep(1000 * time.Millisecond)
	c.Stop()

	// 等待记录写入
	time.Sleep(200 * time.Millisecond)

	// 清理24小时前的记录
	deleted, err := c.CleanupHistory(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("Failed to cleanup history: %v", err)
	}

	if deleted == 0 {
		t.Error("Expected to delete old records")
	}

	// 验证旧记录已删除
	oldRecords, err := c.QueryHistory(history.RecordFilter{
		TaskID: "old-task",
	})
	if err != nil {
		t.Fatalf("Failed to query old records: %v", err)
	}
	if len(oldRecords) > 0 {
		t.Error("Old records should have been deleted")
	}

	// 验证新记录仍然存在
	newRecords, err := c.QueryHistory(history.RecordFilter{
		TaskID: "new-task",
	})
	if err != nil {
		t.Fatalf("Failed to query new records: %v", err)
	}
	if len(newRecords) == 0 {
		t.Error("New records should still exist")
	}
}

// TestHistoryWithoutRecorder 测试未启用历史记录器时的行为
func TestHistoryWithoutRecorder(t *testing.T) {
	c := New() // 不启用历史记录器

	err := c.Schedule("test-task", "@every 1s", func(ctx context.Context) {})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer c.Stop()

	time.Sleep(1500 * time.Millisecond)

	// 尝试查询历史记录（应该返回错误）
	_, err = c.QueryHistory(history.RecordFilter{})
	if err == nil {
		t.Error("Expected error when querying without recorder")
	}

	// 尝试统计记录（应该返回错误）
	_, err = c.CountHistory(history.RecordFilter{})
	if err == nil {
		t.Error("Expected error when counting without recorder")
	}

	// 尝试清理记录（应该返回错误）
	_, err = c.CleanupHistory(time.Now())
	if err == nil {
		t.Error("Expected error when cleaning up without recorder")
	}
}

// TestFileStorage 测试文件存储的基本功能
func TestFileStorage(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "test-storage"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupCronHistoryStorage(t, storage)

	// 测试保存记录
	now := time.Now()
	record := &history.ExecutionRecord{
		ID:         "task1_123",
		TaskID:     "task1",
		StartTime:  now,
		EndTime:    now.Add(1 * time.Second),
		Duration:   1 * time.Second,
		Success:    true,
		RetryCount: 0,
	}

	err = storage.Save(record)
	if err != nil {
		t.Fatalf("Failed to save record: %v", err)
	}

	// 测试查询记录
	records, err := storage.Query(history.RecordFilter{
		TaskID: "task1",
	})
	if err != nil {
		t.Fatalf("Failed to query records: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	if records[0].TaskID != "task1" {
		t.Errorf("Expected TaskID=task1, got %s", records[0].TaskID)
	}

	// 测试统计记录
	count, err := storage.Count(history.RecordFilter{
		TaskID: "task1",
	})
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected count=1, got %d", count)
	}

	// 测试删除记录（删除明天之前的所有记录，即包括今天的记录）
	deleted, err := storage.Delete(time.Now().Add(24 * time.Hour))
	if err != nil {
		t.Fatalf("Failed to delete records: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected deleted=1, got %d", deleted)
	}

	// 验证记录已删除
	records, err = storage.Query(history.RecordFilter{
		TaskID: "task1",
	})
	if err != nil {
		t.Fatalf("Failed to query after delete: %v", err)
	}

	if len(records) != 0 {
		t.Error("Records should have been deleted")
	}
}

// TestFileStorageSharding 测试文件存储的分片功能
func TestFileStorageSharding(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "sharding-test"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer cleanupCronHistoryStorage(t, storage)

	// 保存多天的记录
	baseTime := time.Now().Add(-3 * 24 * time.Hour)
	for i := range 4 {
		recordTime := baseTime.Add(time.Duration(i) * 24 * time.Hour)
		record := &history.ExecutionRecord{
			ID:        "task_" + recordTime.Format("20060102"),
			TaskID:    "sharding-task",
			StartTime: recordTime,
			EndTime:   recordTime.Add(1 * time.Second),
			Duration:  1 * time.Second,
			Success:   true,
		}
		err = storage.Save(record)
		if err != nil {
			t.Fatalf("Failed to save record for day %d: %v", i, err)
		}
	}

	// 查询所有记录
	allRecords, err := storage.Query(history.RecordFilter{
		TaskID: "sharding-task",
	})
	if err != nil {
		t.Fatalf("Failed to query all records: %v", err)
	}

	if len(allRecords) != 4 {
		t.Errorf("Expected 4 records, got %d", len(allRecords))
	}

	// 验证文件分片（应该有4个日期文件）
	taskDir := filepath.Join(tmpDir, "sharding-test", "sharding-task")
	files, err := os.ReadDir(taskDir)
	if err != nil {
		t.Fatalf("Failed to read task directory: %v", err)
	}

	if len(files) != 4 {
		t.Errorf("Expected 4 date files, got %d", len(files))
	}
}
