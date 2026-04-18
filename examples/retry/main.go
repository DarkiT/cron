package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/darkit/cron"
)

func main() {
	// 创建调度器
	c := cron.New()

	// 示例 1：固定重试次数
	example1FixedRetry(c)

	// 示例 2：无限重试
	example2InfiniteRetry(c)

	// 示例 3：立即重试
	example3ImmediateRetry(c)

	// 示例 4：重试与超时结合
	example4RetryWithTimeout(c)

	// 启动调度器
	if err := c.Start(); err != nil {
		log.Fatalf("启动调度器失败: %v", err)
	}

	fmt.Println("调度器已启动，按 Ctrl+C 退出...")

	// 等待一段时间查看效果
	time.Sleep(30 * time.Second)

	// 输出统计信息
	printStats(c)

	// 停止调度器
	c.Stop()
	fmt.Println("调度器已停止")
}

// 示例 1：固定重试次数
func example1FixedRetry(c *cron.Cron) {
	attempts := 0
	handler := func(ctx context.Context) {
		attempts++
		fmt.Printf("[固定重试] 第 %d 次尝试\n", attempts)

		// 前 2 次失败，第 3 次成功
		if attempts < 3 {
			panic(fmt.Sprintf("第 %d 次失败", attempts))
		}

		fmt.Println("[固定重试] 执行成功！")
		attempts = 0 // 重置计数器供下次调度使用
	}

	err := c.Schedule("fixed-retry", "@every 10s", handler, cron.JobOptions{
		MaxRetries:    3,               // 最多重试 3 次
		RetryInterval: 1 * time.Second, // 重试间隔 1 秒
		MaxConcurrent: 1,               // 防止并发执行
	})
	if err != nil {
		log.Fatalf("添加固定重试任务失败: %v", err)
	}
}

// 示例 2：无限重试
func example2InfiniteRetry(c *cron.Cron) {
	attempts := 0
	handler := func(ctx context.Context) {
		attempts++
		fmt.Printf("[无限重试] 第 %d 次尝试\n", attempts)

		// 前 5 次失败，第 6 次成功
		if attempts < 6 {
			panic(fmt.Sprintf("第 %d 次失败", attempts))
		}

		fmt.Println("[无限重试] 执行成功！")
		attempts = 0
	}

	err := c.Schedule("infinite-retry", "@every 15s", handler, cron.JobOptions{
		MaxRetries:    -1,                     // 无限重试
		RetryInterval: 500 * time.Millisecond, // 重试间隔 500ms
		MaxConcurrent: 1,
	})
	if err != nil {
		log.Fatalf("添加无限重试任务失败: %v", err)
	}
}

// 示例 3：立即重试
func example3ImmediateRetry(c *cron.Cron) {
	attempts := 0
	handler := func(ctx context.Context) {
		attempts++
		fmt.Printf("[立即重试] 第 %d 次尝试\n", attempts)

		// 前 3 次失败，第 4 次成功
		if attempts < 4 {
			panic(fmt.Sprintf("第 %d 次失败", attempts))
		}

		fmt.Println("[立即重试] 执行成功！")
		attempts = 0
	}

	err := c.Schedule("immediate-retry", "@every 20s", handler, cron.JobOptions{
		MaxRetries:    5, // 最多重试 5 次
		RetryInterval: 0, // 立即重试（不等待）
		MaxConcurrent: 1,
	})
	if err != nil {
		log.Fatalf("添加立即重试任务失败: %v", err)
	}
}

// 示例 4：重试与超时结合
func example4RetryWithTimeout(c *cron.Cron) {
	attempts := 0
	handler := func(ctx context.Context) {
		attempts++
		fmt.Printf("[超时重试] 第 %d 次尝试\n", attempts)

		// 前 2 次模拟超时（执行时间过长）
		if attempts < 3 {
			fmt.Println("[超时重试] 执行耗时操作...")
			time.Sleep(2 * time.Second) // 超过超时设置
			return
		}

		// 第 3 次快速完成
		fmt.Println("[超时重试] 快速完成！")
		attempts = 0
	}

	err := c.Schedule("timeout-retry", "@every 25s", handler, cron.JobOptions{
		Timeout:       1 * time.Second,        // 超时时间 1 秒
		MaxRetries:    5,                      // 最多重试 5 次
		RetryInterval: 500 * time.Millisecond, // 重试间隔 500ms
		MaxConcurrent: 1,
	})
	if err != nil {
		log.Fatalf("添加超时重试任务失败: %v", err)
	}
}

// 打印统计信息
func printStats(c *cron.Cron) {
	fmt.Println("\n========== 任务统计信息 ==========")
	allStats := c.GetAllStats()
	for id, stats := range allStats {
		fmt.Printf("\n任务: %s\n", id)
		fmt.Printf("  调度表达式: %s\n", stats.Schedule)
		fmt.Printf("  总运行次数: %d\n", stats.RunCount)
		fmt.Printf("  成功次数: %d\n", stats.SuccessCount)
		fmt.Printf("  失败次数: %d\n", stats.FailCount)
		fmt.Printf("  重试总次数: %d\n", stats.RetryCount)
		if !stats.LastRun.IsZero() {
			fmt.Printf("  最后运行: %s\n", stats.LastRun.Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Println("=================================")
}
