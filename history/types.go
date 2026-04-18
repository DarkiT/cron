package history

import (
	"time"
)

// ExecutionRecord 任务执行历史记录
type ExecutionRecord struct {
	ID         string        `json:"id"`         // 记录唯一标识（任务ID_时间戳）
	TaskID     string        `json:"taskID"`     // 任务ID
	StartTime  time.Time     `json:"startTime"`  // 开始时间
	EndTime    time.Time     `json:"endTime"`    // 结束时间
	Duration   time.Duration `json:"duration"`   // 执行耗时（纳秒）
	Success    bool          `json:"success"`    // 是否成功
	RetryCount int           `json:"retryCount"` // 重试次数
	Error      string        `json:"error"`      // 错误信息（如果失败）
}

// RecordFilter 查询过滤器
type RecordFilter struct {
	TaskID      string     // 任务ID（为空则查询所有任务）
	StartTime   *time.Time // 开始时间范围（可选）
	EndTime     *time.Time // 结束时间范围（可选）
	SuccessOnly bool       // 仅查询成功的记录
	FailedOnly  bool       // 仅查询失败的记录
	Limit       int        // 返回记录数限制（0表示不限制）
	Offset      int        // 偏移量（分页）
}

// Storage 定义历史记录存储接口
type Storage interface {
	// Save 保存一条执行记录
	Save(record *ExecutionRecord) error

	// Query 根据过滤器查询记录
	Query(filter RecordFilter) ([]*ExecutionRecord, error)

	// Count 统计符合条件的记录数量
	Count(filter RecordFilter) (int, error)

	// Delete 删除指定时间范围之前的记录（用于清理旧数据）
	Delete(before time.Time) (int, error)

	// Close 关闭存储连接
	Close() error
}

// Recorder 定义历史记录器接口
type Recorder interface {
	// Record 记录任务执行结果
	Record(taskID string, startTime, endTime time.Time, success bool, retryCount int, err error)

	// Query 查询历史记录
	Query(filter RecordFilter) ([]*ExecutionRecord, error)

	// Count 统计记录数量
	Count(filter RecordFilter) (int, error)

	// Cleanup 清理指定时间之前的历史记录
	Cleanup(before time.Time) (int, error)

	// Close 关闭记录器
	Close() error
}
