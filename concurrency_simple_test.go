package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestSimpleConcurrency 简单的并发测试
func TestSimpleConcurrency(t *testing.T) {
	// Case 1: 无限并发
	c := New()
	defer c.Stop()

	var runningCountUnlimited int64
	var maxConcurrentUnlimited int64

	unlimitedJob := &ConcurrencyTestJob{
		handler: func(ctx context.Context) {
			current := atomic.AddInt64(&runningCountUnlimited, 1)

			for {
				max := atomic.LoadInt64(&maxConcurrentUnlimited)
				if current > max {
					if atomic.CompareAndSwapInt64(&maxConcurrentUnlimited, max, current) {
						break
					}
				} else {
					break
				}
			}

			time.Sleep(1 * time.Second)
			atomic.AddInt64(&runningCountUnlimited, -1)
		},
	}

	err := c.ScheduleJob("unlimited", "@every 100ms", unlimitedJob, JobOptions{
		MaxConcurrent: 0,
		Async:         true,
	})
	if err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	c.Start()
	time.Sleep(400 * time.Millisecond)
	c.Stop()

	maxReached := atomic.LoadInt64(&maxConcurrentUnlimited)
	t.Logf("无限并发测试: 最大并发数 = %d", maxReached)

	if maxReached < 2 {
		t.Errorf("无限并发模式失败: 期望最大并发 >= 2, 实际 = %d", maxReached)
	}

	// Case 2: 串行执行
	c2 := New()
	defer c2.Stop()

	var runningCountSerial int64
	var maxConcurrentSerial int64

	serialJob := &ConcurrencyTestJob{
		handler: func(ctx context.Context) {
			current := atomic.AddInt64(&runningCountSerial, 1)

			for {
				max := atomic.LoadInt64(&maxConcurrentSerial)
				if current > max {
					if atomic.CompareAndSwapInt64(&maxConcurrentSerial, max, current) {
						break
					}
				} else {
					break
				}
			}

			time.Sleep(120 * time.Millisecond)
			atomic.AddInt64(&runningCountSerial, -1)
		},
	}

	err = c2.ScheduleJob("serial", "@every 100ms", serialJob, JobOptions{
		MaxConcurrent: 1,
		Async:         true,
	})
	if err != nil {
		t.Fatalf("添加串行任务失败: %v", err)
	}

	c2.Start()
	time.Sleep(400 * time.Millisecond)
	c2.Stop()

	maxReached = atomic.LoadInt64(&maxConcurrentSerial)
	t.Logf("串行执行测试: 最大并发数 = %d", maxReached)

	if maxReached != 1 {
		t.Errorf("串行执行模式失败: 期望最大并发 = 1, 实际 = %d", maxReached)
	}
}
