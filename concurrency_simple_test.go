package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestSimpleConcurrency 简单的并发测试
func TestSimpleConcurrency(t *testing.T) {
	c := New()
	defer c.Stop()

	var runningCount int64
	var maxConcurrent int64

	testJob := &ConcurrencyTestJob{
		handler: func(ctx context.Context) {
			current := atomic.AddInt64(&runningCount, 1)

			// 记录最大并发数
			for {
				max := atomic.LoadInt64(&maxConcurrent)
				if current > max {
					if atomic.CompareAndSwapInt64(&maxConcurrent, max, current) {
						break
					}
				} else {
					break
				}
			}

			// 任务持续1秒
			time.Sleep(1 * time.Second)
			atomic.AddInt64(&runningCount, -1)
		},
	}

	// 测试无限并发 (MaxConcurrent = 0)
	err := c.ScheduleJob("unlimited", "*/1 * * * * *", testJob, JobOptions{
		MaxConcurrent: 0,
		Async:         true,
	})
	if err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	c.Start()

	// 等待足够长时间让多个任务重叠执行
	time.Sleep(3 * time.Second)

	maxReached := atomic.LoadInt64(&maxConcurrent)
	t.Logf("无限并发测试: 最大并发数 = %d", maxReached)

	// 无限并发应该允许多个任务同时运行
	if maxReached < 2 {
		t.Errorf("无限并发模式失败: 期望最大并发 >= 2, 实际 = %d", maxReached)
	}

	c.Stop()

	// 重置计数器测试串行执行
	atomic.StoreInt64(&runningCount, 0)
	atomic.StoreInt64(&maxConcurrent, 0)

	c2 := New()
	defer c2.Stop()

	err = c2.ScheduleJob("serial", "*/1 * * * * *", testJob, JobOptions{
		MaxConcurrent: 1,
		Async:         true,
	})
	if err != nil {
		t.Fatalf("添加串行任务失败: %v", err)
	}

	c2.Start()
	time.Sleep(3 * time.Second)
	c2.Stop()

	maxReached = atomic.LoadInt64(&maxConcurrent)
	t.Logf("串行执行测试: 最大并发数 = %d", maxReached)

	// 串行执行应该最大并发数为1
	if maxReached != 1 {
		t.Errorf("串行执行模式失败: 期望最大并发 = 1, 实际 = %d", maxReached)
	}
}
