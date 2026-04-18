package history

import (
	"fmt"
	"sync"
	"time"
)

// HistoryRecorder 历史记录器实现
type HistoryRecorder struct {
	storage Storage
	queue   chan *ExecutionRecord
	wg      sync.WaitGroup
	opsWg   sync.WaitGroup
	closed  bool
	mu      sync.Mutex
	logger  Logger // 可选的日志记录器，用于记录存储失败等非致命错误
}

// RecorderOption 定义 HistoryRecorder 的配置选项
type RecorderOption func(*HistoryRecorder)

// WithRecorderLogger 设置历史记录器的日志记录器
// logger 为 nil 时，HistoryRecorder 将静默忽略存储失败（向后兼容）
func WithRecorderLogger(logger Logger) RecorderOption {
	return func(hr *HistoryRecorder) {
		hr.logger = logger
	}
}

// NewHistoryRecorder 创建历史记录器
// 支持可选的配置选项，例如 WithRecorderLogger
//
// 示例：
//
//	recorder := NewHistoryRecorder(storage)  // 不带日志
//	recorder := NewHistoryRecorder(storage, WithRecorderLogger(logger))  // 带日志
func NewHistoryRecorder(storage Storage, opts ...RecorderOption) *HistoryRecorder {
	if storage == nil {
		panic("history storage cannot be nil")
	}

	recorder := &HistoryRecorder{
		storage: storage,
		queue:   make(chan *ExecutionRecord, 100), // 缓冲队列，避免阻塞
	}

	// 应用所有选项
	for _, opt := range opts {
		opt(recorder)
	}

	// 启动异步写入协程
	recorder.wg.Add(1)
	go recorder.writeLoop()

	return recorder
}

// Record 记录任务执行结果（异步）
func (hr *HistoryRecorder) Record(taskID string, startTime, endTime time.Time, success bool, retryCount int, err error) {
	hr.mu.Lock()
	if hr.closed {
		hr.mu.Unlock()
		return
	}
	hr.opsWg.Add(1)

	// 生成记录ID
	recordID := fmt.Sprintf("%s_%d", taskID, startTime.UnixNano())

	// 构建记录
	record := &ExecutionRecord{
		ID:         recordID,
		TaskID:     taskID,
		StartTime:  startTime,
		EndTime:    endTime,
		Duration:   endTime.Sub(startTime),
		Success:    success,
		RetryCount: retryCount,
	}

	if err != nil {
		record.Error = err.Error()
	}

	// 异步写入队列
	select {
	case hr.queue <- record:
		hr.mu.Unlock()
		hr.opsWg.Done()
		// 成功入队
	default:
		hr.mu.Unlock()
		defer hr.opsWg.Done()
		// 队列已满，同步写入（避免丢失数据）
		if err := hr.storage.Save(record); err != nil {
			// 记录存储失败错误，但不阻塞调用方（nil-safe + panic-safe）
			func() {
				defer func() {
					if r := recover(); r != nil {
						// 防止 logger 实现 panic 导致程序崩溃
						// 静默忽略，保持调度器稳定性
					}
				}()
				if hr.logger != nil {
					hr.logger.Warn("队列已满，同步保存历史记录失败",
						"taskID", record.TaskID,
						"recordID", record.ID,
						"error", err.Error())
				}
			}()
		}
	}
}

// Query 查询历史记录
func (hr *HistoryRecorder) Query(filter RecordFilter) ([]*ExecutionRecord, error) {
	return hr.storage.Query(filter)
}

// Count 统计记录数量
func (hr *HistoryRecorder) Count(filter RecordFilter) (int, error) {
	return hr.storage.Count(filter)
}

// Cleanup 清理指定时间之前的历史记录
func (hr *HistoryRecorder) Cleanup(before time.Time) (int, error) {
	return hr.storage.Delete(before)
}

// Close 关闭记录器
func (hr *HistoryRecorder) Close() error {
	hr.mu.Lock()
	if hr.closed {
		hr.mu.Unlock()
		return nil
	}
	hr.closed = true
	hr.mu.Unlock()

	// 关闭队列
	close(hr.queue)

	// 等待所有 Record 调用结束，确保不会向已关闭队列发送
	hr.opsWg.Wait()

	// 等待写入完成
	hr.wg.Wait()

	// 关闭存储
	return hr.storage.Close()
}

// writeLoop 异步写入循环
func (hr *HistoryRecorder) writeLoop() {
	defer hr.wg.Done()

	for record := range hr.queue {
		if err := hr.storage.Save(record); err != nil {
			// 记录异步保存失败错误（nil-safe + panic-safe）
			func() {
				defer func() {
					if r := recover(); r != nil {
						// 防止 logger 实现 panic 导致程序崩溃
						// 静默忽略，保持调度器稳定性
					}
				}()
				if hr.logger != nil {
					hr.logger.Warn("异步保存历史记录失败",
						"taskID", record.TaskID,
						"recordID", record.ID,
						"error", err.Error())
				}
			}()
		}
	}
}
