package jobs

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/darkit/cron"
)

// MonitorJob 系统监控任务
type MonitorJob struct {
	name    string
	counter *int64
}

var monitorCounter = int64(0)

func init() {
	// 在包初始化时自动注册任务
	job := &MonitorJob{
		name:    "系统监控",
		counter: &monitorCounter,
	}
	// 使用RegisterJob，永远不会panic
	cron.RegisterJob(job)
}

func (j *MonitorJob) Schedule() string {
	return "*/4 * * * * *" // 每4秒执行一次
}

func (j *MonitorJob) Name() string {
	return "auto-monitor"
}

func (j *MonitorJob) Run(ctx context.Context) error {
	count := atomic.AddInt64(j.counter, 1)
	fmt.Printf("[%s] %s 检查系统状态 (次数: %d)\n",
		time.Now().Format("15:04:05"), j.name, count)

	// 模拟监控检查
	select {
	case <-time.After(150 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
