package jobs

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/darkit/cron"
)

// BackupJob 自动备份任务
type BackupJob struct {
	name    string
	counter *int64
}

var backupCounter = int64(0)

func init() {
	// 在包初始化时自动注册任务
	job := &BackupJob{
		name:    "自动备份",
		counter: &backupCounter,
	}
	// 使用RegisterJob，永远不会panic
	cron.RegisterJob(job)
}

func (j *BackupJob) Name() string {
	return "auto-backup"
}

func (j *BackupJob) Schedule() string {
	return "*/3 * * * * *" // 每3秒执行一次
}

func (j *BackupJob) Run(ctx context.Context) error {
	count := atomic.AddInt64(j.counter, 1)
	fmt.Printf("[%s] %s 执行备份操作 (次数: %d)\n",
		time.Now().Format("15:04:05"), j.name, count)

	// 模拟备份操作
	select {
	case <-time.After(200 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
