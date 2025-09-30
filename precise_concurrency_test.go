package cron

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPreciseConcurrencyControl 精确测试并发控制逻辑
func TestPreciseConcurrencyControl(t *testing.T) {
	tests := []struct {
		name              string
		maxConcurrent     int
		simultaneousTasks int
		expectExecuted    int
		expectSkipped     int
		description       string
	}{
		{
			name:              "无限并发",
			maxConcurrent:     0,
			simultaneousTasks: 5,
			expectExecuted:    5, // 所有任务都应该执行
			expectSkipped:     0,
			description:       "MaxConcurrent=0应该允许所有任务并发执行",
		},
		{
			name:              "串行执行",
			maxConcurrent:     1,
			simultaneousTasks: 5,
			expectExecuted:    1, // 只有1个任务执行，4个被跳过
			expectSkipped:     4,
			description:       "MaxConcurrent=1应该只允许1个任务执行，其他立即放弃",
		},
		{
			name:              "限制2个并发",
			maxConcurrent:     2,
			simultaneousTasks: 5,
			expectExecuted:    2, // 只有2个任务执行，3个被跳过
			expectSkipped:     3,
			description:       "MaxConcurrent=2应该只允许2个任务执行，其他立即放弃",
		},
		{
			name:              "限制3个并发",
			maxConcurrent:     3,
			simultaneousTasks: 5,
			expectExecuted:    3, // 只有3个任务执行，2个被跳过
			expectSkipped:     2,
			description:       "MaxConcurrent=3应该只允许3个任务执行，其他立即放弃",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := newScheduler()
			scheduler.logger = NewDefaultLogger()

			var executedCount int64
			var maxConcurrent int64
			var currentRunning int64

			task := &Task{
				ID:       "precise-test",
				Schedule: "* * * * * *",
				Handler: func(ctx context.Context) {
					executed := atomic.AddInt64(&executedCount, 1)
					current := atomic.AddInt64(&currentRunning, 1)

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

					t.Logf("任务 %d 开始执行, 当前并发: %d", executed, current)

					// 任务执行时间足够长以便观察并发
					time.Sleep(100 * time.Millisecond)

					atomic.AddInt64(&currentRunning, -1)
					t.Logf("任务 %d 执行完成", executed)
				},
				Options: JobOptions{MaxConcurrent: tt.maxConcurrent},
			}

			// 添加任务
			err := scheduler.addTask(task)
			if err != nil {
				t.Fatalf("添加任务失败: %v", err)
			}

			runner := scheduler.tasks[task.ID]

			// 同时触发多个任务执行
			var wg sync.WaitGroup
			startCh := make(chan struct{})

			for i := 0; i < tt.simultaneousTasks; i++ {
				wg.Add(1)
				go func(taskNum int) {
					defer wg.Done()
					<-startCh // 等待同时开始信号

					// 直接调用executeTask，如果被跳过会在日志中显示
					scheduler.executeTask(runner)
				}(i)
			}

			// 等待所有任务尝试完成
			time.Sleep(50 * time.Millisecond)

			// 同时开始所有任务
			close(startCh)

			// 等待所有goroutine完成
			wg.Wait()

			executed := atomic.LoadInt64(&executedCount)
			skipped := int64(tt.simultaneousTasks) - executed // 计算跳过数量
			maxReached := atomic.LoadInt64(&maxConcurrent)

			t.Logf("%s: 执行=%d, 跳过=%d, 最大并发=%d",
				tt.description, executed, skipped, maxReached)

			// 验证执行数量
			if int(executed) != tt.expectExecuted {
				t.Errorf("执行数量错误: 期望=%d, 实际=%d", tt.expectExecuted, executed)
			}

			// 验证跳过数量
			if int(skipped) != tt.expectSkipped {
				t.Errorf("跳过数量错误: 期望=%d, 实际=%d", tt.expectSkipped, skipped)
			}

			// 验证最大并发数
			if tt.maxConcurrent > 0 && maxReached > int64(tt.maxConcurrent) {
				t.Errorf("并发控制失效: 期望最大并发<=%d, 实际=%d", tt.maxConcurrent, maxReached)
			}

			// 验证无限并发的情况
			if tt.maxConcurrent == 0 && maxReached != int64(tt.expectExecuted) {
				t.Errorf("无限并发失效: 期望并发=%d, 实际=%d", tt.expectExecuted, maxReached)
			}
		})
	}
}

// TestConcurrencyControlWithRealScheduler 使用真实调度器测试并发控制
func TestConcurrencyControlWithRealScheduler(t *testing.T) {
	tests := []struct {
		name                string
		maxConcurrent       int
		taskDuration        time.Duration
		expectMaxConcurrent int64
	}{
		{
			name:                "无限并发",
			maxConcurrent:       0,
			taskDuration:        300 * time.Millisecond,
			expectMaxConcurrent: -1, // -1表示不限制，具体值取决于调度频率
		},
		{
			name:                "串行执行",
			maxConcurrent:       1,
			taskDuration:        300 * time.Millisecond,
			expectMaxConcurrent: 1,
		},
		{
			name:                "限制2个并发",
			maxConcurrent:       2,
			taskDuration:        300 * time.Millisecond,
			expectMaxConcurrent: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			defer c.Stop()

			var currentRunning int64
			var maxConcurrent int64
			var totalExecuted int64

			testJob := &ConcurrencyTestJob{
				handler: func(ctx context.Context) {
					atomic.AddInt64(&totalExecuted, 1)
					current := atomic.AddInt64(&currentRunning, 1)

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

					// 模拟任务执行时间
					time.Sleep(tt.taskDuration)
					atomic.AddInt64(&currentRunning, -1)
				},
			}

			// 添加任务 - 每100ms执行一次
			err := c.ScheduleJob("real-concurrency-test", "@every 100ms", testJob, JobOptions{
				MaxConcurrent: tt.maxConcurrent,
				Async:         true, // 异步执行以观察真实并发
			})
			if err != nil {
				t.Fatalf("添加任务失败: %v", err)
			}

			c.Start()

			// 运行足够长时间让多个任务重叠执行
			time.Sleep(2 * time.Second)

			c.Stop()

			executed := atomic.LoadInt64(&totalExecuted)
			maxReached := atomic.LoadInt64(&maxConcurrent)

			t.Logf("%s: 总执行次数=%d, 最大并发数=%d", tt.name, executed, maxReached)

			// 验证最大并发数
			if tt.expectMaxConcurrent > 0 && maxReached > tt.expectMaxConcurrent {
				t.Errorf("并发控制失效: 期望最大并发<=%d, 实际=%d", tt.expectMaxConcurrent, maxReached)
			}

			if tt.expectMaxConcurrent > 0 && maxReached != tt.expectMaxConcurrent {
				t.Errorf("并发控制异常: 期望最大并发=%d, 实际=%d", tt.expectMaxConcurrent, maxReached)
			}

			// 确保至少有任务执行
			if executed == 0 {
				t.Error("没有任务被执行")
			}
		})
	}
}
