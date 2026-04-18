package dashboard

import (
	"time"

	"github.com/darkit/cron/history"
)

// TaskInfo 任务信息
type TaskInfo struct {
	ID            string            `json:"id"`            // 任务ID
	Schedule      string            `json:"schedule"`      // 调度表达式
	NextRun       time.Time         `json:"nextRun"`       // 下次执行时间
	IsRunning     bool              `json:"isRunning"`     // 是否正在运行
	RunCount      int64             `json:"runCount"`      // 运行次数
	SuccessCount  int64             `json:"successCount"`  // 成功次数
	FailCount     int64             `json:"failCount"`     // 失败次数
	RetryCount    int64             `json:"retryCount"`    // 重试次数
	SkippedCount  int64             `json:"skippedCount"`  // 因并发限制被跳过次数
	PauseUntil    time.Time         `json:"pauseUntil"`    // 暂停到期时间（熔断或手动）
	MisfirePolicy string            `json:"misfirePolicy"` // Misfire 策略
	LastRunTime   time.Time         `json:"lastRunTime"`   // 上次运行时间
	LastRunStatus string            `json:"lastRunStatus"` // 上次运行状态
	LastError     string            `json:"lastError"`     // 最后一次错误
	Descriptions  map[string]string `json:"descriptions"`  // 任务描述信息
	Labels        map[string]string `json:"labels"`        // 任务标签
}

// StatsInfo 统计信息
type StatsInfo struct {
	TotalTasks     int     `json:"totalTasks"`     // 总任务数
	RunningTasks   int     `json:"runningTasks"`   // 运行中任务数
	TotalRuns      int64   `json:"totalRuns"`      // 总运行次数
	SuccessRuns    int64   `json:"successRuns"`    // 成功运行次数
	FailedRuns     int64   `json:"failedRuns"`     // 失败运行次数
	TotalRetries   int64   `json:"totalRetries"`   // 总重试次数
	SuccessRate    float64 `json:"successRate"`    // 成功率
	AvgDuration    string  `json:"avgDuration"`    // 平均执行时长
	TotalDuration  string  `json:"totalDuration"`  // 总执行时长
	HistoryRecords int     `json:"historyRecords"` // 历史记录数
}

// HistoryFilter 历史记录过滤器
type HistoryFilter struct {
	TaskID      string     `json:"taskId,omitempty"`
	SuccessOnly bool       `json:"successOnly,omitempty"`
	FailedOnly  bool       `json:"failedOnly,omitempty"`
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Limit       int        `json:"limit,omitempty"`
	Offset      int        `json:"offset,omitempty"`
}

// HistoryResponse 历史记录响应
type HistoryResponse struct {
	Records    []history.ExecutionRecord `json:"records"`
	Total      int                       `json:"total"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"pageSize"`
	TotalPages int                       `json:"totalPages"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// SuccessResponse 成功响应
type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
