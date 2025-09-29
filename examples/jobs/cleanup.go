package jobs

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/darkit/cron"
)

// CleanupJob 自动清理任务
type CleanupJob struct {
	name    string
	counter *int64
}

var cleanupCounter = int64(0)

func init() {
	// 在包初始化时自动注册任务
	job := &CleanupJob{
		name:    "自动清理",
		counter: &cleanupCounter,
	}
	// 使用RegisterJob，永远不会panic
	cron.RegisterJob(job)
}

func (j *CleanupJob) Schedule() string {
	return "*/5 * * * * *" // 每5秒执行一次
}

func (j *CleanupJob) Name() string {
	return "auto-cleanup"
}

func (j *CleanupJob) Run(ctx context.Context) error {
	count := atomic.AddInt64(j.counter, 1)
	fmt.Printf("[%s] %s 执行清理操作 (次数: %d)\n",
		time.Now().Format("15:04:05"), j.name, count)

	// 模拟清理操作
	select {
	case <-time.After(100 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
