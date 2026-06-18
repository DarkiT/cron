package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrencyControl 测试并发控制功能
func TestConcurrencyControl(t *testing.T) {
	tests := []struct {
		name          string
		maxConcurrent int
		taskDuration  time.Duration
		expectSkipped bool
		description   string
	}{
		{
			name:          "无限并发",
			maxConcurrent: 0,
			taskDuration:  80 * time.Millisecond,
			expectSkipped: false,
			description:   "MaxConcurrent=0应该允许无限并发",
		},
		{
			name:          "串行执行",
			maxConcurrent: 1,
			taskDuration:  80 * time.Millisecond,
			expectSkipped: true,
			description:   "MaxConcurrent=1应该强制串行执行",
		},
		{
			name:          "限制并发数",
			maxConcurrent: 2,
			taskDuration:  80 * time.Millisecond,
			expectSkipped: true,
			description:   "MaxConcurrent=2应该最多允许2个并发",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			defer c.Stop()

			var runningCount int64
			var maxRunning int64
			var totalExecuted int64

			job := func(ctx context.Context) {
				atomic.AddInt64(&totalExecuted, 1)

				current := atomic.AddInt64(&runningCount, 1)

				// 更新最大并发数记录
				for {
					max := atomic.LoadInt64(&maxRunning)
					if current > max {
						if atomic.CompareAndSwapInt64(&maxRunning, max, current) {
							break
						}
					} else {
						break
					}
				}

				// 模拟任务执行时间
				time.Sleep(tt.taskDuration)
				atomic.AddInt64(&runningCount, -1)
			}

			// 添加任务 - 每50ms执行一次，缩短测试时间
			err := c.ScheduleJob("test-concurrent", "@every 50ms", &ConcurrencyTestJob{handler: job}, JobOptions{
				MaxConcurrent: tt.maxConcurrent,
				Async:         true,
			})
			if err != nil {
				t.Fatalf("添加任务失败: %v", err)
			}

			// 启动调度器
			if err := c.Start(); err != nil {
				t.Fatalf("启动调度器失败: %v", err)
			}

			// 运行足够长时间以触发多次执行
			time.Sleep(400 * time.Millisecond)

			// 停止调度器，等待正在执行的任务完成
			c.Stop()

			executed := atomic.LoadInt64(&totalExecuted)
			maxConcurrentReached := atomic.LoadInt64(&maxRunning)

			t.Logf("%s: 总执行次数=%d, 最大并发数=%d", tt.description, executed, maxConcurrentReached)

			// 验证结果
			switch tt.maxConcurrent {
			case 0:
				// 无限并发：应该能达到较高的并发数
				if maxConcurrentReached < 2 {
					t.Errorf("无限并发模式下最大并发数应该 >= 2，实际为 %d", maxConcurrentReached)
				}
			case 1:
				// 串行执行：最大并发数应该为1
				if maxConcurrentReached != 1 {
					t.Errorf("串行执行模式下最大并发数应该为1，实际为 %d", maxConcurrentReached)
				}
			default:
				// 限制并发：最大并发数应该不超过设定值
				if maxConcurrentReached > int64(tt.maxConcurrent) {
					t.Errorf("并发控制失效：最大并发数应该 <= %d，实际为 %d", tt.maxConcurrent, maxConcurrentReached)
				}
			}

			// 验证是否有任务执行
			if executed == 0 {
				t.Error("没有任务被执行")
			}
		})
	}
}

// ConcurrencyTestJob 测试用的Job实现
type ConcurrencyTestJob struct {
	handler func(ctx context.Context)
}

func (j *ConcurrencyTestJob) Name() string {
	return "concurrency-test-job"
}

func (j *ConcurrencyTestJob) Run(ctx context.Context) error {
	if j.handler != nil {
		j.handler(ctx)
	}
	return nil
}

// TestConcurrentTaskExecution 测试具体的并发任务执行场景
func TestConcurrentTaskExecution(t *testing.T) {
	testCases := []struct {
		name      string
		maxConcur int
		expectMax int64
	}{
		{"无限并发", 0, 5}, // 期望能达到较高并发
		{"串行执行", 1, 1}, // 期望最大并发为1
		{"限制3个", 3, 3}, // 期望最大并发为3
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			defer c.Stop()

			var concurrentCount int64
			var maxConcurrent int64

			blockingTask := func(ctx context.Context) {
				current := atomic.AddInt64(&concurrentCount, 1)

				// 更新最大并发记录
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

				// 阻塞500ms
				select {
				case <-time.After(500 * time.Millisecond):
				case <-ctx.Done():
				}

				atomic.AddInt64(&concurrentCount, -1)
			}

			err := c.ScheduleJob("concurrent-test", "@every 50ms", &ConcurrencyTestJob{handler: blockingTask}, JobOptions{
				MaxConcurrent: tc.maxConcur,
				Async:         true,
			})
			if err != nil {
				t.Fatalf("添加任务失败: %v", err)
			}

			if err := c.Start(); err != nil {
				t.Fatalf("启动调度器失败: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
			c.Stop()

			maxReached := atomic.LoadInt64(&maxConcurrent)
			t.Logf("%s: 实际最大并发数 = %d, 期望最大值 = %d", tc.name, maxReached, tc.expectMax)

			switch tc.maxConcur {
			case 0:
				if maxReached < tc.expectMax {
					t.Errorf("无限并发未达到预期并发度，实际=%d，期望>=%d", maxReached, tc.expectMax)
				}
			case 1:
				if maxReached != tc.expectMax {
					t.Errorf("串行执行最大并发应为1，实际=%d", maxReached)
				}
			default:
				if maxReached > tc.expectMax {
					t.Errorf("最大并发应 <= %d，实际=%d", tc.expectMax, maxReached)
				}
			}
		})
	}
}
