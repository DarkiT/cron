package cron

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrentTrigger 测试同时触发多个任务执行时的并发控制
func TestConcurrentTrigger(t *testing.T) {
	c := New()
	defer c.Stop()

	var runningCount int64
	var maxConcurrent int64
	var totalStarted int64
	var totalCompleted int64

	testJob := &ConcurrencyTestJob{
		handler: func(ctx context.Context) {
			atomic.AddInt64(&totalStarted, 1)

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

			// 任务持续500ms
			time.Sleep(500 * time.Millisecond)
			atomic.AddInt64(&runningCount, -1)
			atomic.AddInt64(&totalCompleted, 1)
		},
	}

	// 测试MaxConcurrent=1的情况
	err := c.ScheduleJob("serial-test", "@every 10ms", testJob, JobOptions{
		MaxConcurrent: 1,
		Async:         false, // 同步执行更容易观察并发控制
	})
	if err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	c.Start()

	// 运行1秒，10ms间隔意味着应该触发100次，但只有非重叠的能执行
	time.Sleep(1 * time.Second)

	c.Stop()

	started := atomic.LoadInt64(&totalStarted)
	completed := atomic.LoadInt64(&totalCompleted)
	maxReached := atomic.LoadInt64(&maxConcurrent)

	t.Logf("串行执行测试: 启动次数=%d, 完成次数=%d, 最大并发=%d", started, completed, maxReached)

	// 验证：最大并发应该为1
	if maxReached != 1 {
		t.Errorf("串行执行失败: 期望最大并发=1, 实际=%d", maxReached)
	}

	// 验证：由于任务持续500ms，1秒内最多能完成2个任务
	// 而且中间的很多触发应该被跳过
	if started > completed*2 {
		t.Logf("正确行为: 启动次数(%d) 少于可能的触发次数，说明后续任务被正确跳过", started)
	}
}

// TestManualConcurrentExecution 手动触发并发执行测试
func TestManualConcurrentExecution(t *testing.T) {
	scheduler := newScheduler()
	scheduler.logger = NewDefaultLogger()

	var runningCount int64
	var maxConcurrent int64
	var executedCount int64

	task := &Task{
		ID:       "manual-test",
		Schedule: "* * * * * *",
		Handler: func(ctx context.Context) {
			atomic.AddInt64(&executedCount, 1)
			current := atomic.AddInt64(&runningCount, 1)

			// 记录最大并发
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

			time.Sleep(200 * time.Millisecond)
			atomic.AddInt64(&runningCount, -1)
		},
		Options: JobOptions{MaxConcurrent: 1},
	}

	// 添加任务但不启动调度器
	err := scheduler.addTask(task)
	if err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	runner := scheduler.tasks[task.ID]

	// 同时触发多个执行
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scheduler.executeTask(runner)
		}()
	}

	wg.Wait()

	executed := atomic.LoadInt64(&executedCount)
	maxReached := atomic.LoadInt64(&maxConcurrent)

	t.Logf("手动并发触发测试: 执行次数=%d, 最大并发=%d", executed, maxReached)

	// MaxConcurrent=1时，同时触发5次，应该只有1个执行，其他4个被跳过
	if executed != 1 {
		t.Errorf("并发控制失败: 期望执行1次, 实际执行%d次", executed)
	}

	if maxReached != 1 {
		t.Errorf("并发控制失败: 期望最大并发=1, 实际=%d", maxReached)
	}
}
