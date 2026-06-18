package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockLogger 用于测试的日志记录器
type mockLogger struct {
	warnings []logEntry
}

type logEntry struct {
	msg           string
	keysAndValues []any
}

func (m *mockLogger) Warn(msg string, keysAndValues ...any) {
	m.warnings = append(m.warnings, logEntry{
		msg:           msg,
		keysAndValues: keysAndValues,
	})
}

func (m *mockLogger) HasWarning(msg string) bool {
	for _, w := range m.warnings {
		if w.msg == msg {
			return true
		}
	}
	return false
}

func (m *mockLogger) WarningCount() int {
	return len(m.warnings)
}

func TestNewFileStorage(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	if storage == nil {
		t.Error("存储实例不应为 nil")
	}

	// 验证目录已创建
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("基础目录未创建")
	}
}

func TestFileStorageSave(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	now := time.Now()
	record := &ExecutionRecord{
		ID:         "test-task_123",
		TaskID:     "test-task",
		StartTime:  now,
		EndTime:    now.Add(1 * time.Second),
		Duration:   1 * time.Second,
		Success:    true,
		RetryCount: 0,
	}

	err = storage.Save(record)
	if err != nil {
		t.Fatalf("保存记录失败: %v", err)
	}

	// 验证文件已创建
	dateStr := now.Format("2006-01-02")
	expectedFile := filepath.Join(tmpDir, "test-task", dateStr+".jsonl")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("记录文件未创建: %s", expectedFile)
	}
}

func TestFileStorageRejectsInvalidTaskID(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	now := time.Now()
	for _, taskID := range []string{"../escape", "task/sub", `task\\sub`, ".", ".."} {
		err := storage.Save(&ExecutionRecord{
			ID:        "record-1",
			TaskID:    taskID,
			StartTime: now,
			EndTime:   now.Add(time.Second),
			Duration:  time.Second,
			Success:   true,
		})
		if err == nil {
			t.Fatalf("expected invalid task id %q to be rejected", taskID)
		}
	}
}

func TestFileStorageSaveNil(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	err = storage.Save(nil)
	if err == nil {
		t.Error("保存 nil 记录应该返回错误")
	}
}

func TestFileStorageQuery(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 保存多条记录
	now := time.Now()
	records := []*ExecutionRecord{
		{
			ID:        "task1_1",
			TaskID:    "task1",
			StartTime: now,
			EndTime:   now.Add(1 * time.Second),
			Duration:  1 * time.Second,
			Success:   true,
		},
		{
			ID:        "task1_2",
			TaskID:    "task1",
			StartTime: now.Add(2 * time.Second),
			EndTime:   now.Add(3 * time.Second),
			Duration:  1 * time.Second,
			Success:   false,
			Error:     "test error",
		},
		{
			ID:        "task2_1",
			TaskID:    "task2",
			StartTime: now,
			EndTime:   now.Add(1 * time.Second),
			Duration:  1 * time.Second,
			Success:   true,
		},
	}

	for _, record := range records {
		if err := storage.Save(record); err != nil {
			t.Fatalf("保存记录失败: %v", err)
		}
	}

	// 查询所有记录
	allRecords, err := storage.Query(RecordFilter{})
	if err != nil {
		t.Fatalf("查询所有记录失败: %v", err)
	}
	if len(allRecords) != 3 {
		t.Errorf("期望 3 条记录，得到 %d 条", len(allRecords))
	}

	// 查询特定任务
	task1Records, err := storage.Query(RecordFilter{TaskID: "task1"})
	if err != nil {
		t.Fatalf("查询 task1 记录失败: %v", err)
	}
	if len(task1Records) != 2 {
		t.Errorf("期望 task1 有 2 条记录，得到 %d 条", len(task1Records))
	}

	// 查询成功的记录
	successRecords, err := storage.Query(RecordFilter{SuccessOnly: true})
	if err != nil {
		t.Fatalf("查询成功记录失败: %v", err)
	}
	if len(successRecords) != 2 {
		t.Errorf("期望 2 条成功记录，得到 %d 条", len(successRecords))
	}

	// 查询失败的记录
	failedRecords, err := storage.Query(RecordFilter{FailedOnly: true})
	if err != nil {
		t.Fatalf("查询失败记录失败: %v", err)
	}
	if len(failedRecords) != 1 {
		t.Errorf("期望 1 条失败记录，得到 %d 条", len(failedRecords))
	}
	if failedRecords[0].Error != "test error" {
		t.Errorf("期望错误信息为 'test error'，得到 '%s'", failedRecords[0].Error)
	}
}

func TestFileStorageQueryWithTimeRange(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 保存不同时间的记录
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	records := []*ExecutionRecord{
		{
			ID:        "task_old",
			TaskID:    "task",
			StartTime: yesterday,
			EndTime:   yesterday.Add(1 * time.Second),
			Duration:  1 * time.Second,
			Success:   true,
		},
		{
			ID:        "task_now",
			TaskID:    "task",
			StartTime: now,
			EndTime:   now.Add(1 * time.Second),
			Duration:  1 * time.Second,
			Success:   true,
		},
	}

	for _, record := range records {
		if err := storage.Save(record); err != nil {
			t.Fatalf("保存记录失败: %v", err)
		}
	}

	// 查询今天及以后的记录
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	recentRecords, err := storage.Query(RecordFilter{
		StartTime: &todayStart,
		EndTime:   &tomorrow,
	})
	if err != nil {
		t.Fatalf("查询时间范围记录失败: %v", err)
	}

	if len(recentRecords) != 1 {
		t.Errorf("期望 1 条今天的记录，得到 %d 条", len(recentRecords))
	}
}

func TestFileStorageQueryWithPagination(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 保存多条记录
	now := time.Now()
	for i := range 10 {
		record := &ExecutionRecord{
			ID:        "task_" + string(rune(i)),
			TaskID:    "task",
			StartTime: now.Add(time.Duration(i) * time.Second),
			EndTime:   now.Add(time.Duration(i+1) * time.Second),
			Duration:  1 * time.Second,
			Success:   true,
		}
		if err := storage.Save(record); err != nil {
			t.Fatalf("保存记录失败: %v", err)
		}
	}

	// 查询前 5 条
	firstPage, err := storage.Query(RecordFilter{
		TaskID: "task",
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("查询第一页失败: %v", err)
	}
	if len(firstPage) != 5 {
		t.Errorf("期望 5 条记录，得到 %d 条", len(firstPage))
	}

	// 查询接下来的 5 条
	secondPage, err := storage.Query(RecordFilter{
		TaskID: "task",
		Offset: 5,
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("查询第二页失败: %v", err)
	}
	if len(secondPage) != 5 {
		t.Errorf("期望 5 条记录，得到 %d 条", len(secondPage))
	}

	// 验证不重复
	if firstPage[0].ID == secondPage[0].ID {
		t.Error("分页结果不应重复")
	}
}

func TestFileStorageCount(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 保存记录
	now := time.Now()
	for i := range 5 {
		record := &ExecutionRecord{
			ID:        "task_" + string(rune(i)),
			TaskID:    "task",
			StartTime: now,
			EndTime:   now.Add(1 * time.Second),
			Duration:  1 * time.Second,
			Success:   i%2 == 0,
		}
		if err := storage.Save(record); err != nil {
			t.Fatalf("保存记录失败: %v", err)
		}
	}

	// 统计所有记录
	totalCount, err := storage.Count(RecordFilter{TaskID: "task"})
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if totalCount != 5 {
		t.Errorf("期望 5 条记录，得到 %d 条", totalCount)
	}

	// 统计成功的记录
	successCount, err := storage.Count(RecordFilter{
		TaskID:      "task",
		SuccessOnly: true,
	})
	if err != nil {
		t.Fatalf("统计成功记录失败: %v", err)
	}
	if successCount != 3 {
		t.Errorf("期望 3 条成功记录，得到 %d 条", successCount)
	}
}

func TestFileStorageDelete(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 保存旧记录和新记录
	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)

	oldRecord := &ExecutionRecord{
		ID:        "old_task",
		TaskID:    "task",
		StartTime: oldTime,
		EndTime:   oldTime.Add(1 * time.Second),
		Duration:  1 * time.Second,
		Success:   true,
	}

	newRecord := &ExecutionRecord{
		ID:        "new_task",
		TaskID:    "task",
		StartTime: now,
		EndTime:   now.Add(1 * time.Second),
		Duration:  1 * time.Second,
		Success:   true,
	}

	if err := storage.Save(oldRecord); err != nil {
		t.Fatalf("保存旧记录失败: %v", err)
	}
	if err := storage.Save(newRecord); err != nil {
		t.Fatalf("保存新记录失败: %v", err)
	}

	// 删除 24 小时前的记录
	deleted, err := storage.Delete(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("删除记录失败: %v", err)
	}
	if deleted == 0 {
		t.Error("期望删除至少 1 条记录")
	}

	// 验证旧记录已删除
	allRecords, err := storage.Query(RecordFilter{})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}

	for _, record := range allRecords {
		if record.ID == "old_task" {
			t.Error("旧记录应该已被删除")
		}
	}

	// 验证新记录仍然存在
	found := false
	for _, record := range allRecords {
		if record.ID == "new_task" {
			found = true
			break
		}
	}
	if !found {
		t.Error("新记录应该仍然存在")
	}
}

func TestFileStorageSharding(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 保存跨越多天的记录
	baseTime := time.Now().Add(-3 * 24 * time.Hour)
	for i := range 4 {
		recordTime := baseTime.Add(time.Duration(i) * 24 * time.Hour)
		record := &ExecutionRecord{
			ID:        "task_day_" + recordTime.Format("20060102"),
			TaskID:    "sharding-task",
			StartTime: recordTime,
			EndTime:   recordTime.Add(1 * time.Second),
			Duration:  1 * time.Second,
			Success:   true,
		}
		if err := storage.Save(record); err != nil {
			t.Fatalf("保存第 %d 天的记录失败: %v", i, err)
		}
	}

	// 验证文件分片
	taskDir := filepath.Join(tmpDir, "sharding-task")
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		t.Fatalf("读取任务目录失败: %v", err)
	}

	if len(entries) != 4 {
		t.Errorf("期望 4 个日期文件，得到 %d 个", len(entries))
	}

	// 查询所有记录
	allRecords, err := storage.Query(RecordFilter{TaskID: "sharding-task"})
	if err != nil {
		t.Fatalf("查询所有记录失败: %v", err)
	}
	if len(allRecords) != 4 {
		t.Errorf("期望 4 条记录，得到 %d 条", len(allRecords))
	}
}

// TestFileStorageWithLogger 测试带日志记录器的存储
func TestFileStorageWithLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logger := &mockLogger{}
	storage, err := NewFileStorage(tmpDir, WithLogger(logger))
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	if storage.logger == nil {
		t.Error("logger 应该被设置")
	}

	// 验证初始时没有警告
	if logger.WarningCount() != 0 {
		t.Errorf("初始警告数应该为 0，得到 %d", logger.WarningCount())
	}
}

// TestFileStorageWithoutLogger 测试不带日志记录器的存储（向后兼容）
func TestFileStorageWithoutLogger(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	if storage.logger != nil {
		t.Error("logger 应该为 nil")
	}

	// 测试正常功能
	record := &ExecutionRecord{
		ID:        "test_task",
		TaskID:    "task",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Second),
		Duration:  1 * time.Second,
		Success:   true,
	}

	if err := storage.Save(record); err != nil {
		t.Fatalf("保存记录失败: %v", err)
	}

	records, err := storage.Query(RecordFilter{TaskID: "task"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("期望 1 条记录，得到 %d 条", len(records))
	}
}

// TestFileStorageLoggerOnCorruptedFile 测试损坏文件时的日志记录
func TestFileStorageLoggerOnCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	logger := &mockLogger{}
	storage, err := NewFileStorage(tmpDir, WithLogger(logger))
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 创建一个带有损坏 JSON 的文件
	taskDir := filepath.Join(tmpDir, "task1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("创建任务目录失败: %v", err)
	}

	dateFile := filepath.Join(taskDir, time.Now().Format("2006-01-02")+".jsonl")
	corruptedContent := `{"id":"valid_record","taskID":"task1","startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-01T00:00:01Z","duration":1000000000,"success":true,"retryCount":0,"error":""}
{invalid json line
{"id":"another_valid","taskID":"task1","startTime":"2024-01-01T00:00:02Z","endTime":"2024-01-01T00:00:03Z","duration":1000000000,"success":true,"retryCount":0,"error":""}
`
	if err := os.WriteFile(dateFile, []byte(corruptedContent), 0o644); err != nil {
		t.Fatalf("创建损坏文件失败: %v", err)
	}

	// 查询记录应该跳过损坏的行并记录警告
	records, err := storage.Query(RecordFilter{TaskID: "task1"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}

	// 应该得到 2 条有效记录
	if len(records) != 2 {
		t.Errorf("期望 2 条有效记录，得到 %d 条", len(records))
	}

	// 应该有 1 条警告（关于无效 JSON）
	if !logger.HasWarning("解析历史记录 JSON 失败，跳过该行") {
		t.Error("应该记录 JSON 解析失败的警告")
	}
}

// TestFileStorageLoggerOnMissingDirectory 测试目录不存在时的日志记录
func TestFileStorageLoggerOnMissingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logger := &mockLogger{}
	storage, err := NewFileStorage(tmpDir, WithLogger(logger))
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 查询不存在的任务不应产生警告
	records, err := storage.Query(RecordFilter{TaskID: "nonexistent"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("期望 0 条记录，得到 %d 条", len(records))
	}

	// 不应有警告（任务目录不存在是正常情况）
	if logger.WarningCount() != 0 {
		t.Errorf("不应有警告，得到 %d 条", logger.WarningCount())
	}
}

// TestFileStorageLoggerOnDelete 测试删除操作中的日志记录
func TestFileStorageLoggerOnDelete(t *testing.T) {
	tmpDir := t.TempDir()
	logger := &mockLogger{}
	storage, err := NewFileStorage(tmpDir, WithLogger(logger))
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 保存一条正常记录
	oldTime := time.Now().Add(-48 * time.Hour)
	record := &ExecutionRecord{
		ID:        "old_task",
		TaskID:    "task",
		StartTime: oldTime,
		EndTime:   oldTime.Add(1 * time.Second),
		Duration:  1 * time.Second,
		Success:   true,
	}

	if err := storage.Save(record); err != nil {
		t.Fatalf("保存记录失败: %v", err)
	}

	// 保存一个更旧的记录（带损坏的 JSON 行）
	taskDir := filepath.Join(tmpDir, "task")
	veryOldDate := oldTime.Add(-48*time.Hour).Format("2006-01-02") + ".jsonl"
	corruptedFile := filepath.Join(taskDir, veryOldDate)
	corruptedContent := `{"id":"valid","taskID":"task","startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-01T00:00:01Z","duration":1000000000,"success":true,"retryCount":0,"error":""}
{invalid json line
`
	if err := os.WriteFile(corruptedFile, []byte(corruptedContent), 0o644); err != nil {
		t.Fatalf("创建包含损坏 JSON 的文件失败: %v", err)
	}

	// 删除旧记录（删除 24 小时前的记录）
	deleted, err := storage.Delete(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("删除记录失败: %v", err)
	}

	// 应该删除了至少 1 条记录（包括损坏文件中的有效行）
	if deleted < 1 {
		t.Errorf("期望删除至少 1 条记录，实际删除 %d 条", deleted)
	}

	// 验证损坏文件已被删除
	if _, err := os.Stat(corruptedFile); err == nil {
		t.Error("损坏的文件应该已被删除")
	}
}

// TestFileStorageNilSafeLogger 测试 nil logger 的安全性
func TestFileStorageNilSafeLogger(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir) // 不带 logger
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	// 创建一个带有损坏 JSON 的文件
	taskDir := filepath.Join(tmpDir, "task1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("创建任务目录失败: %v", err)
	}

	dateFile := filepath.Join(taskDir, time.Now().Format("2006-01-02")+".jsonl")
	if err := os.WriteFile(dateFile, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatalf("创建损坏文件失败: %v", err)
	}

	// 查询应该不会 panic，即使 logger 为 nil
	records, err := storage.Query(RecordFilter{TaskID: "task1"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}

	// 应该得到 0 条记录（损坏的行被跳过）
	if len(records) != 0 {
		t.Errorf("期望 0 条记录，得到 %d 条", len(records))
	}
}

// TestFileStorageMultipleOptions 测试多个选项组合
func TestFileStorageMultipleOptions(t *testing.T) {
	tmpDir := t.TempDir()
	logger := &mockLogger{}

	// 可以多次调用 WithLogger（最后一个生效）
	storage, err := NewFileStorage(tmpDir, WithLogger(logger))
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			t.Fatalf("关闭存储失败: %v", err)
		}
	}()

	if storage.logger != logger {
		t.Error("logger 应该被正确设置")
	}
}

// BenchmarkFileStorageWithLogger 测试带日志的性能影响
func BenchmarkFileStorageWithLogger(b *testing.B) {
	tmpDir := b.TempDir()
	logger := &mockLogger{}
	storage, err := NewFileStorage(tmpDir, WithLogger(logger))
	if err != nil {
		b.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			b.Fatalf("关闭存储失败: %v", err)
		}
	}()

	record := &ExecutionRecord{
		ID:        "bench_task",
		TaskID:    "task",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Second),
		Duration:  1 * time.Second,
		Success:   true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = storage.Save(record)
	}
}

// BenchmarkFileStorageWithoutLogger 测试不带日志的性能（基准）
func BenchmarkFileStorageWithoutLogger(b *testing.B) {
	tmpDir := b.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		b.Fatalf("创建存储失败: %v", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			b.Fatalf("关闭存储失败: %v", err)
		}
	}()

	record := &ExecutionRecord{
		ID:        "bench_task",
		TaskID:    "task",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Second),
		Duration:  1 * time.Second,
		Success:   true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = storage.Save(record)
	}
}
