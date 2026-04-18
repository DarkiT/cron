package history

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestNewHistoryRecorderPanicsOnNilStorage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when storage is nil")
		}
	}()

	_ = NewHistoryRecorder(nil)
}

func TestNewHistoryRecorder(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	if recorder == nil {
		t.Error("记录器实例不应为 nil")
	}
	defer recorder.Close()
}

func TestHistoryRecorderRecord(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 记录成功执行
	startTime := time.Now()
	endTime := startTime.Add(1 * time.Second)
	recorder.Record("test-task", startTime, endTime, true, 0, nil)

	// 等待异步写入完成
	time.Sleep(200 * time.Millisecond)

	// 查询记录
	records, err := recorder.Query(RecordFilter{TaskID: "test-task"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("期望 1 条记录，得到 %d 条", len(records))
	}

	record := records[0]
	if record.TaskID != "test-task" {
		t.Errorf("期望 TaskID=test-task，得到 %s", record.TaskID)
	}
	if !record.Success {
		t.Error("期望 Success=true")
	}
	if record.RetryCount != 0 {
		t.Errorf("期望 RetryCount=0，得到 %d", record.RetryCount)
	}
}

func TestHistoryRecorderRecordWithError(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 记录失败执行
	startTime := time.Now()
	endTime := startTime.Add(1 * time.Second)
	testError := fmt.Errorf("test error")
	recorder.Record("fail-task", startTime, endTime, false, 3, testError)

	// 等待异步写入完成
	time.Sleep(200 * time.Millisecond)

	// 查询记录
	records, err := recorder.Query(RecordFilter{TaskID: "fail-task"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("期望 1 条记录，得到 %d 条", len(records))
	}

	record := records[0]
	if record.Success {
		t.Error("期望 Success=false")
	}
	if record.RetryCount != 3 {
		t.Errorf("期望 RetryCount=3，得到 %d", record.RetryCount)
	}
	if record.Error != "test error" {
		t.Errorf("期望 Error='test error'，得到 '%s'", record.Error)
	}
}

func TestHistoryRecorderQuery(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 记录多个任务
	now := time.Now()
	recorder.Record("task1", now, now.Add(1*time.Second), true, 0, nil)
	recorder.Record("task2", now, now.Add(1*time.Second), false, 2, fmt.Errorf("error"))
	recorder.Record("task1", now.Add(2*time.Second), now.Add(3*time.Second), true, 0, nil)

	// 等待异步写入完成
	time.Sleep(200 * time.Millisecond)

	// 查询所有记录
	allRecords, err := recorder.Query(RecordFilter{})
	if err != nil {
		t.Fatalf("查询所有记录失败: %v", err)
	}
	if len(allRecords) != 3 {
		t.Errorf("期望 3 条记录，得到 %d 条", len(allRecords))
	}

	// 查询 task1 的记录
	task1Records, err := recorder.Query(RecordFilter{TaskID: "task1"})
	if err != nil {
		t.Fatalf("查询 task1 记录失败: %v", err)
	}
	if len(task1Records) != 2 {
		t.Errorf("期望 task1 有 2 条记录，得到 %d 条", len(task1Records))
	}

	// 查询成功的记录
	successRecords, err := recorder.Query(RecordFilter{SuccessOnly: true})
	if err != nil {
		t.Fatalf("查询成功记录失败: %v", err)
	}
	if len(successRecords) != 2 {
		t.Errorf("期望 2 条成功记录，得到 %d 条", len(successRecords))
	}
}

func TestHistoryRecorderCount(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 记录多条记录
	now := time.Now()
	for i := 0; i < 5; i++ {
		recorder.Record("count-task", now, now.Add(1*time.Second), true, 0, nil)
	}

	// 等待异步写入完成
	time.Sleep(200 * time.Millisecond)

	// 统计记录数量
	count, err := recorder.Count(RecordFilter{TaskID: "count-task"})
	if err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if count != 5 {
		t.Errorf("期望 5 条记录，得到 %d 条", count)
	}
}

func TestHistoryRecorderCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 记录旧记录和新记录
	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)

	recorder.Record("cleanup-task", oldTime, oldTime.Add(1*time.Second), true, 0, nil)
	recorder.Record("cleanup-task", now, now.Add(1*time.Second), true, 0, nil)

	// 等待异步写入完成
	time.Sleep(200 * time.Millisecond)

	// 清理 24 小时前的记录
	deleted, err := recorder.Cleanup(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("清理记录失败: %v", err)
	}
	if deleted == 0 {
		t.Error("期望删除至少 1 条记录")
	}

	// 验证只剩下新记录
	records, err := recorder.Query(RecordFilter{TaskID: "cleanup-task"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("期望剩余 1 条记录，得到 %d 条", len(records))
	}
}

func TestHistoryRecorderClose(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)

	// 记录一些数据
	now := time.Now()
	recorder.Record("close-task", now, now.Add(1*time.Second), true, 0, nil)

	// 关闭记录器
	err = recorder.Close()
	if err != nil {
		t.Fatalf("关闭记录器失败: %v", err)
	}

	// 关闭后尝试记录（应该不会 panic）
	recorder.Record("after-close", now, now.Add(1*time.Second), true, 0, nil)

	// 验证之前的记录已写入
	time.Sleep(100 * time.Millisecond)
	records, err := storage.Query(RecordFilter{TaskID: "close-task"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("期望 1 条记录，得到 %d 条", len(records))
	}
}

func TestHistoryRecorderConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 并发记录
	now := time.Now()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(index int) {
			taskID := fmt.Sprintf("concurrent-task-%d", index)
			recorder.Record(taskID, now, now.Add(1*time.Second), true, 0, nil)
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 等待异步写入完成
	time.Sleep(300 * time.Millisecond)

	// 验证所有记录都已写入
	allRecords, err := recorder.Query(RecordFilter{})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if len(allRecords) != 10 {
		t.Errorf("期望 10 条记录，得到 %d 条", len(allRecords))
	}
}

func TestHistoryRecorderQueueFull(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 快速写入大量记录（超过队列缓冲）
	now := time.Now()
	for i := 0; i < 200; i++ {
		recorder.Record("queue-task", now, now.Add(1*time.Second), true, 0, nil)
	}

	// 等待写入完成
	time.Sleep(500 * time.Millisecond)

	// 验证所有记录都已写入（包括队列满时的同步写入）
	count, err := recorder.Count(RecordFilter{TaskID: "queue-task"})
	if err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if count != 200 {
		t.Errorf("期望 200 条记录，得到 %d 条", count)
	}
}

func TestHistoryRecorderRecordCloseConcurrentDoesNotPanic(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	recorder := NewHistoryRecorder(storage)

	var writers sync.WaitGroup
	panicCh := make(chan interface{}, 1)

	for i := 0; i < 8; i++ {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			defer func() {
				if r := recover(); r != nil {
					select {
					case panicCh <- r:
					default:
					}
				}
			}()

			for j := 0; j < 100; j++ {
				now := time.Now()
				recorder.Record(fmt.Sprintf("task-%d", index), now, now.Add(time.Millisecond), true, 0, nil)
			}
		}(i)
	}

	time.Sleep(10 * time.Millisecond)
	if err := recorder.Close(); err != nil {
		t.Fatalf("关闭记录器失败: %v", err)
	}

	writers.Wait()

	select {
	case r := <-panicCh:
		t.Fatalf("Record/Close 并发不应 panic: %v", r)
	default:
	}
}

// panicLogger 用于测试panic保护的日志记录器
type panicLogger struct{}

func (p *panicLogger) Warn(msg string, keysAndValues ...interface{}) {
	panic("logger panic for testing")
}

// TestRecorderWithLogger 测试 WithRecorderLogger 功能和panic保护
func TestRecorderWithLogger(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	// 测试1：验证正常logger工作
	logger := &mockLogger{}
	recorder := NewHistoryRecorder(storage, WithRecorderLogger(logger))

	now := time.Now()
	for i := 0; i < 10; i++ {
		recorder.Record("test-task", now, now.Add(1*time.Second), true, 0, nil)
	}

	time.Sleep(100 * time.Millisecond)
	recorder.Close()

	// 测试2：验证panic保护 - logger panic不应导致程序崩溃
	tmpDir2 := t.TempDir()
	storage2, err := NewFileStorage(tmpDir2)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage2.Close()

	// 使用会panic的logger
	panicingLogger := &panicLogger{}
	recorder2 := NewHistoryRecorder(storage2, WithRecorderLogger(panicingLogger))
	defer recorder2.Close()

	// 删除目录以触发存储错误，进而触发logger调用
	os.RemoveAll(tmpDir2)

	// 快速填满队列，触发同步写入失败（会调用logger，但logger会panic）
	// panic应该被捕获，程序不应崩溃
	for i := 0; i < 150; i++ {
		recorder2.Record("fail-task", now, now.Add(1*time.Second), true, 0, nil)
	}

	// 等待异步处理（如果没有panic保护，这里会崩溃）
	time.Sleep(300 * time.Millisecond)

	// 如果程序没有崩溃，测试通过
	t.Log("Panic protection working: logger panic did not crash the recorder")
}

// TestRecorderWithoutLogger 测试不带日志记录器的情况（向后兼容）
func TestRecorderWithoutLogger(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}
	defer storage.Close()

	// 不带日志创建记录器
	recorder := NewHistoryRecorder(storage)
	defer recorder.Close()

	// 正常记录应该工作
	now := time.Now()
	recorder.Record("test-task", now, now.Add(1*time.Second), true, 0, nil)

	time.Sleep(100 * time.Millisecond)

	records, err := recorder.Query(RecordFilter{TaskID: "test-task"})
	if err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("期望 1 条记录，得到 %d 条", len(records))
	}
}
