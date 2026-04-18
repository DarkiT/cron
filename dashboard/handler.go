package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darkit/cron"
	"github.com/darkit/cron/history"
)

// Handler Dashboard HTTP 处理器
type Handler struct {
	cron *cron.Cron
}

func taskIDFromRequest(r *http.Request) string {
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		return id
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "tasks" && i+1 < len(parts) {
			return strings.TrimSpace(parts[i+1])
		}
	}

	return ""
}

// NewHandler 创建新的处理器
func NewHandler(c *cron.Cron) *Handler {
	return &Handler{cron: c}
}

// writeJSON 写入 JSON 响应
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError 写入错误响应
func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
		Code:    status,
	})
}

// GetTasks 获取所有任务列表
func (h *Handler) GetTasks(w http.ResponseWriter, r *http.Request) {
	labelKey := r.URL.Query().Get("labelKey")
	labelVal := r.URL.Query().Get("labelVal")

	taskIDs := h.cron.List()
	tasks := make([]TaskInfo, 0, len(taskIDs))

	for _, id := range taskIDs {
		info := h.getTaskInfo(id)
		if info == nil {
			continue
		}
		if labelKey != "" {
			if info.Labels == nil || info.Labels[labelKey] != labelVal {
				continue
			}
		}
		tasks = append(tasks, *info)
	}

	h.writeJSON(w, http.StatusOK, tasks)
}

// GetTask 获取单个任务详情
func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	taskID := taskIDFromRequest(r)

	info := h.getTaskInfo(taskID)
	if info == nil {
		h.writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	h.writeJSON(w, http.StatusOK, info)
}

// GetStats 获取统计信息
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	allStats := h.cron.GetAllStats()

	stats := StatsInfo{
		TotalTasks: len(allStats),
	}

	for _, stat := range allStats {
		if stat.IsRunning {
			stats.RunningTasks++
		}
		stats.TotalRuns += stat.RunCount
		stats.SuccessRuns += stat.SuccessCount
		stats.FailedRuns += stat.FailCount
		stats.TotalRetries += stat.RetryCount
	}

	// 计算成功率
	if stats.TotalRuns > 0 {
		stats.SuccessRate = float64(stats.SuccessRuns) / float64(stats.TotalRuns) * 100
	}

	var totalDuration time.Duration
	for _, stat := range allStats {
		totalDuration += stat.TotalDuration
	}
	if stats.TotalRuns > 0 {
		stats.AvgDuration = h.formatDuration(totalDuration / time.Duration(stats.TotalRuns))
		stats.TotalDuration = h.formatDuration(totalDuration)
	} else {
		stats.AvgDuration = "0s"
		stats.TotalDuration = "0s"
	}

	if historyCount, err := h.cron.CountHistory(history.RecordFilter{}); err == nil {
		stats.HistoryRecords = historyCount
	}

	h.writeJSON(w, http.StatusOK, stats)
}

// GetHistory 获取历史记录
func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	// 解析查询参数
	filter := history.RecordFilter{}

	if taskID := r.URL.Query().Get("taskId"); taskID != "" {
		filter.TaskID = taskID
	}

	if r.URL.Query().Get("successOnly") == "true" {
		filter.SuccessOnly = true
	}

	if r.URL.Query().Get("failedOnly") == "true" {
		filter.FailedOnly = true
	}

	if startTimeStr := r.URL.Query().Get("startTime"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			filter.StartTime = &startTime
		}
	}

	if endTimeStr := r.URL.Query().Get("endTime"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			filter.EndTime = &endTime
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 50 // 默认每页50条
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	// 查询历史记录
	recordsPtr, err := h.cron.QueryHistory(filter)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "Failed to query history: "+err.Error())
		return
	}

	// 转换为非指针切片
	records := make([]history.ExecutionRecord, len(recordsPtr))
	for i, rec := range recordsPtr {
		records[i] = *rec
	}

	// 统计总数
	totalCount, err := h.cron.CountHistory(history.RecordFilter{
		TaskID:      filter.TaskID,
		SuccessOnly: filter.SuccessOnly,
		FailedOnly:  filter.FailedOnly,
		StartTime:   filter.StartTime,
		EndTime:     filter.EndTime,
	})
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "Failed to count history: "+err.Error())
		return
	}

	// 计算分页信息
	pageSize := filter.Limit
	totalPages := (totalCount + pageSize - 1) / pageSize
	page := filter.Offset/pageSize + 1

	response := HistoryResponse{
		Records:    records,
		Total:      totalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// RemoveTask 移除任务
func (h *Handler) RemoveTask(w http.ResponseWriter, r *http.Request) {
	taskID := taskIDFromRequest(r)

	err := h.cron.Remove(taskID)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Message: "Task removed successfully",
	})
}

// RunTaskNow 立即触发任务
func (h *Handler) RunTaskNow(w http.ResponseWriter, r *http.Request) {
	taskID := taskIDFromRequest(r)

	if err := h.cron.RunNow(taskID); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{Message: "Task triggered"})
}

// PauseTask 暂停任务调度
func (h *Handler) PauseTask(w http.ResponseWriter, r *http.Request) {
	taskID := taskIDFromRequest(r)

	if err := h.cron.Pause(taskID); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{Message: "Task paused"})
}

// ResumeTask 恢复任务调度
func (h *Handler) ResumeTask(w http.ResponseWriter, r *http.Request) {
	taskID := taskIDFromRequest(r)

	if err := h.cron.Resume(taskID); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{Message: "Task resumed"})
}

// UnfuseTask 恢复因历史兼容路由而触发的任务恢复操作。
// Deprecated: 使用 /resume；/unfuse 仅为向后兼容保留。
func (h *Handler) UnfuseTask(w http.ResponseWriter, r *http.Request) {
	taskID := taskIDFromRequest(r)

	if err := h.cron.Resume(taskID); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{Message: "Task unfused (deprecated alias; compatibility alias of /resume)"})
}

type updateScheduleRequest struct {
	Schedule string `json:"schedule"`
}

// UpdateTaskSchedule 更新任务调度表达式
func (h *Handler) UpdateTaskSchedule(w http.ResponseWriter, r *http.Request) {
	taskID := taskIDFromRequest(r)

	var req updateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Schedule) == "" {
		h.writeError(w, http.StatusBadRequest, "schedule is required")
		return
	}

	if err := h.cron.Update(taskID, req.Schedule); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{Message: "Schedule updated"})
}

// getTaskInfo 获取任务信息（内部方法）
func (h *Handler) getTaskInfo(taskID string) *TaskInfo {
	// 获取下次运行时间
	nextRun, err := h.cron.NextRun(taskID)
	if err != nil {
		return nil
	}

	// 获取统计信息
	stats, ok := h.cron.GetStats(taskID)
	if !ok {
		return nil
	}

	info := &TaskInfo{
		ID:            taskID,
		Schedule:      stats.Schedule,
		NextRun:       nextRun,
		IsRunning:     stats.IsRunning,
		RunCount:      stats.RunCount,
		SuccessCount:  stats.SuccessCount,
		FailCount:     stats.FailCount,
		RetryCount:    stats.RetryCount,
		SkippedCount:  stats.SkippedCount,
		PauseUntil:    stats.PauseUntil,
		MisfirePolicy: stats.MisfirePolicy,
		LastRunTime:   stats.LastRun,
		Labels:        stats.Labels,
		LastError:     stats.LastError,
		Descriptions:  make(map[string]string),
	}

	if stats.HasLastResult {
		if stats.LastRunSuccess {
			info.LastRunStatus = "success"
		} else {
			info.LastRunStatus = "failed"
		}
	}

	return info
}

// formatDuration 格式化时长为用户友好的字符串
func (h *Handler) formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	// 小于 1 秒，显示毫秒
	if d < time.Second {
		ms := d.Milliseconds()
		return fmt.Sprintf("%dms", ms)
	}

	// 小于 1 分钟，显示秒（保留2位小数）
	if d < time.Minute {
		seconds := float64(d) / float64(time.Second)
		return fmt.Sprintf("%.2fs", seconds)
	}

	// 小于 1 小时，显示分钟和秒
	if d < time.Hour {
		minutes := d / time.Minute
		seconds := (d % time.Minute) / time.Second
		if seconds > 0 {
			return fmt.Sprintf("%dm%ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}

	// 1 小时及以上，显示小时、分钟、秒
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	seconds := (d % time.Minute) / time.Second

	if seconds > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dh", hours)
}
