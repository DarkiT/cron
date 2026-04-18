package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

func normalizeStorageTaskID(taskID string) (string, error) {
	normalized := strings.TrimSpace(taskID)
	if normalized == "" {
		return "", fmt.Errorf("task id cannot be empty")
	}
	if normalized == "." || normalized == ".." {
		return "", fmt.Errorf("task id %q is invalid", normalized)
	}
	if strings.ContainsAny(normalized, `/\\`) {
		return "", fmt.Errorf("task id %q cannot contain path separators", normalized)
	}
	return normalized, nil
}

// Logger 日志接口，用于记录存储层的非致命错误
// 实现此接口的类型可以接收文件读取、解析等操作中的错误信息
type Logger interface {
	// Warn 记录警告级别的日志
	Warn(msg string, keysAndValues ...interface{})
}

// FileStorage 基于文件系统的历史记录存储
// 存储结构：<baseDir>/<taskID>/<date>.jsonl
// 每行一条 JSON 记录，避免数组追加的锁竞争和尾部损坏
type FileStorage struct {
	baseDir string
	logger  Logger // 可选的日志记录器，用于记录非致命错误
	mu      sync.RWMutex
}

// Option 定义 FileStorage 的配置选项
type Option func(*FileStorage)

// WithLogger 设置日志记录器
// logger 为 nil 时，FileStorage 将静默忽略非致命错误（向后兼容）
func WithLogger(logger Logger) Option {
	return func(fs *FileStorage) {
		fs.logger = logger
	}
}

// NewFileStorage 创建文件存储实例
// 支持可选的配置选项，例如 WithLogger
//
// 示例：
//
//	storage, err := NewFileStorage("/path/to/data")  // 不带日志
//	storage, err := NewFileStorage("/path/to/data", WithLogger(logger))  // 带日志
func NewFileStorage(baseDir string, opts ...Option) (*FileStorage, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	fs := &FileStorage{baseDir: baseDir}

	// 应用所有选项
	for _, opt := range opts {
		opt(fs)
	}

	return fs, nil
}

// Save 保存一条执行记录（JSONL 追加）
func (fs *FileStorage) Save(record *ExecutionRecord) error {
	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	normalizedTaskID, err := normalizeStorageTaskID(record.TaskID)
	if err != nil {
		return err
	}

	dateStr := record.StartTime.Format("2006-01-02")
	taskDir := filepath.Join(fs.baseDir, normalizedTaskID)
	filePath := filepath.Join(taskDir, dateStr+".jsonl")

	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return fmt.Errorf("failed to create task directory: %w", err)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to append history record: %w", err)
	}
	return nil
}

// Query 根据过滤器查询记录
func (fs *FileStorage) Query(filter RecordFilter) ([]*ExecutionRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var allRecords []*ExecutionRecord

	taskDirs, err := fs.getTaskDirs(filter.TaskID)
	if err != nil {
		return nil, err
	}

	for _, taskDir := range taskDirs {
		dateFiles, err := fs.getDateFiles(taskDir, filter.StartTime, filter.EndTime)
		if err != nil {
			// 记录目录读取错误，但继续处理其他任务目录（panic-safe）
			func() {
				defer func() {
					if r := recover(); r != nil {
						// 防止 logger 实现 panic 导致程序崩溃
						// 静默忽略，保持存储层稳定性
					}
				}()
				if fs.logger != nil {
					fs.logger.Warn("无法读取任务目录下的日期文件",
						"taskDir", taskDir,
						"error", err.Error())
				}
			}()
			continue
		}

		for _, dateFile := range dateFiles {
			records, err := fs.readRecordsFromFile(dateFile)
			if err != nil {
				// 记录文件读取错误，但继续处理其他文件（panic-safe）
				func() {
					defer func() {
						if r := recover(); r != nil {
							// 防止 logger 实现 panic 导致程序崩溃
							// 静默忽略，保持存储层稳定性
						}
					}()
					if fs.logger != nil {
						fs.logger.Warn("读取历史记录文件失败",
							"file", dateFile,
							"error", err.Error())
					}
				}()
				continue
			}
			for _, record := range records {
				if fs.matchFilter(record, filter) {
					allRecords = append(allRecords, record)
				}
			}
		}
	}

	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].StartTime.After(allRecords[j].StartTime)
	})

	if filter.Offset > 0 && filter.Offset < len(allRecords) {
		allRecords = allRecords[filter.Offset:]
	} else if filter.Offset >= len(allRecords) {
		return []*ExecutionRecord{}, nil
	}

	if filter.Limit > 0 && filter.Limit < len(allRecords) {
		allRecords = allRecords[:filter.Limit]
	}

	return allRecords, nil
}

// Count 统计符合条件的记录数量
func (fs *FileStorage) Count(filter RecordFilter) (int, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	taskDirs, err := fs.getTaskDirs(filter.TaskID)
	if err != nil {
		return 0, err
	}

	total := 0
	for _, taskDir := range taskDirs {
		dateFiles, err := fs.getDateFiles(taskDir, filter.StartTime, filter.EndTime)
		if err != nil {
			continue
		}
		for _, dateFile := range dateFiles {
			count, err := fs.countRecordsInFile(dateFile, filter)
			if err != nil {
				continue
			}
			total += count
		}
	}

	return total, nil
}

// Delete 删除指定时间范围之前的记录
func (fs *FileStorage) Delete(before time.Time) (int, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	deletedCount := 0
	beforeDate := before.Format("2006-01-02")

	taskDirs, err := fs.getTaskDirs("")
	if err != nil {
		return 0, err
	}

	for _, taskDir := range taskDirs {
		entries, err := os.ReadDir(taskDir)
		if err != nil {
			// 记录目录读取错误，但继续处理其他任务目录（panic-safe）
			func() {
				defer func() {
					if r := recover(); r != nil {
						// 防止 logger 实现 panic 导致程序崩溃
						// 静默忽略，保持存储层稳定性
					}
				}()
				if fs.logger != nil {
					fs.logger.Warn("无法读取任务目录以进行删除操作",
						"taskDir", taskDir,
						"error", err.Error())
				}
			}()
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			dateStr := strings.TrimSuffix(entry.Name(), ".jsonl")
			if dateStr < beforeDate {
				filePath := filepath.Join(taskDir, entry.Name())
				records, err := fs.readRecordsFromFile(filePath)
				if err == nil {
					deletedCount += len(records)
				} else {
					// 即使读取失败，也记录日志并尝试删除文件（panic-safe）
					func() {
						defer func() {
							if r := recover(); r != nil {
								// 防止 logger 实现 panic 导致程序崩溃
								// 静默忽略，保持存储层稳定性
							}
						}()
						if fs.logger != nil {
							fs.logger.Warn("删除前读取记录数失败，将继续删除文件",
								"file", filePath,
								"error", err.Error())
						}
					}()
				}
				_ = os.Remove(filePath)
			}
		}

		if isEmpty, _ := fs.isDirEmpty(taskDir); isEmpty {
			_ = os.Remove(taskDir)
		}
	}

	return deletedCount, nil
}

// Close 关闭存储连接
func (fs *FileStorage) Close() error { return nil }

// 辅助方法

func (fs *FileStorage) readRecordsFromFile(filePath string) ([]*ExecutionRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []*ExecutionRecord
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec ExecutionRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			// 记录 JSON 解析错误，但继续读取下一行（panic-safe）
			func() {
				defer func() {
					if r := recover(); r != nil {
						// 防止 logger 实现 panic 导致程序崩溃
						// 静默忽略，保持存储层稳定性
					}
				}()
				if fs.logger != nil {
					fs.logger.Warn("解析历史记录 JSON 失败，跳过该行",
						"file", filePath,
						"line", lineNum,
						"error", err.Error())
				}
			}()
			continue
		}
		records = append(records, &rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (fs *FileStorage) getTaskDirs(taskID string) ([]string, error) {
	if taskID != "" {
		normalizedTaskID, err := normalizeStorageTaskID(taskID)
		if err != nil {
			return nil, err
		}
		taskDir := filepath.Join(fs.baseDir, normalizedTaskID)
		if _, err := os.Stat(taskDir); err == nil {
			return []string{taskDir}, nil
		}
		return []string{}, nil
	}

	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(fs.baseDir, entry.Name()))
		}
	}
	return dirs, nil
}

func (fs *FileStorage) countRecordsInFile(filePath string, filter RecordFilter) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec ExecutionRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if fs.matchFilter(&rec, filter) {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func (fs *FileStorage) getDateFiles(taskDir string, startTime, endTime *time.Time) ([]string, error) {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		dateStr := strings.TrimSuffix(entry.Name(), ".jsonl")
		if fs.isDateInRange(dateStr, startTime, endTime) {
			files = append(files, filepath.Join(taskDir, entry.Name()))
		}
	}
	return files, nil
}

func (fs *FileStorage) isDateInRange(dateStr string, startTime, endTime *time.Time) bool {
	if startTime != nil && dateStr < startTime.Format("2006-01-02") {
		return false
	}
	if endTime != nil && dateStr > endTime.Format("2006-01-02") {
		return false
	}
	return true
}

func (fs *FileStorage) matchFilter(record *ExecutionRecord, filter RecordFilter) bool {
	if filter.TaskID != "" && record.TaskID != filter.TaskID {
		return false
	}
	if filter.StartTime != nil && record.StartTime.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && record.StartTime.After(*filter.EndTime) {
		return false
	}
	if filter.SuccessOnly && !record.Success {
		return false
	}
	if filter.FailedOnly && record.Success {
		return false
	}
	return true
}

func (fs *FileStorage) isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}
