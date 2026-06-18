package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/darkit/cron"
	"github.com/darkit/cron/dashboard"
	"github.com/darkit/cron/history"
)

func main() {
	fmt.Println("=== Cron Dashboard 演示 ===")

	// 1. 创建历史记录存储
	homeDir, _ := os.UserHomeDir()
	historyDir := filepath.Join(homeDir, ".cron_history")
	storage, err := history.NewFileStorage(historyDir)
	if err != nil {
		fmt.Printf("创建历史记录存储失败: %v\n", err)
		return
	}
	defer storage.Close()

	// 2. 创建历史记录器
	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		fmt.Printf("创建历史记录器失败: %v\n", err)
		return
	}
	defer recorder.Close()

	// 3. 创建上下文（支持优雅关闭）
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 4. 创建启用历史记录的调度器
	c := cron.New(
		cron.WithHistoryRecorder(recorder),
		cron.WithContext(ctx),
	)

	// 5. 添加示例任务
	fmt.Println("▸ 添加示例任务...")

	// 任务 1：每 5 秒执行的成功任务
	if err := c.Schedule("task-success", "@every 5s", func(ctx context.Context) {
		fmt.Println("  ✓ 成功任务执行")
		time.Sleep(100 * time.Millisecond)
	}); err != nil {
		fmt.Printf("添加 task-success 失败: %v\n", err)
		return
	}

	// 任务 2：每 8 秒执行的可能失败任务（带重试）
	attemptCount := 0
	if err := c.Schedule("task-retry", "@every 8s", func(ctx context.Context) {
		attemptCount++
		if attemptCount%3 == 1 {
			fmt.Println("  ✗ 重试任务失败（将重试）")
			panic("模拟失败")
		}
		fmt.Println("  ✓ 重试任务成功")
		attemptCount = 0
	}, cron.JobOptions{
		MaxRetries:    2,
		RetryInterval: 1 * time.Second,
	}); err != nil {
		fmt.Printf("添加 task-retry 失败: %v\n", err)
		return
	}

	// 任务 3：每 6 秒执行的随机失败任务
	if err := c.Schedule("task-random", "@every 6s", func(ctx context.Context) {
		if rand.Intn(2) == 0 {
			fmt.Println("  ✗ 随机任务失败")
			panic("随机失败")
		}
		fmt.Println("  ✓ 随机任务成功")
	}, cron.JobOptions{
		MaxRetries: 0, // 不重试
	}); err != nil {
		fmt.Printf("添加 task-random 失败: %v\n", err)
		return
	}

	// 任务 4：每 10 秒执行的慢任务
	if err := c.Schedule("task-slow", "@every 10s", func(ctx context.Context) {
		fmt.Println("  ⏳ 慢任务开始执行...")
		time.Sleep(2 * time.Second)
		fmt.Println("  ✓ 慢任务完成")
	}); err != nil {
		fmt.Printf("添加 task-slow 失败: %v\n", err)
		return
	}

	// 任务 5：每分钟执行的定时任务
	if err := c.Schedule("task-hourly", "0 * * * *", func(ctx context.Context) {
		fmt.Println("  ⏰ 每小时任务执行")
	}); err != nil {
		fmt.Printf("添加 task-hourly 失败: %v\n", err)
		return
	}

	// 6. 启动调度器
	if err := c.Start(); err != nil {
		fmt.Printf("启动调度器失败: %v\n", err)
		return
	}
	fmt.Println("▸ 调度器已启动")

	// 7. 启动 Dashboard 服务器
	dashboardServer := dashboard.NewServer(c, ":8080")
	if err := dashboardServer.Start(); err != nil {
		fmt.Printf("启动 Dashboard 失败: %v\n", err)
		return
	}
	defer dashboardServer.Stop()

	fmt.Println("\n=== Dashboard 已启动 ===")
	fmt.Println("请在浏览器中访问: http://localhost:8080")
	fmt.Println("\n功能说明:")
	fmt.Println("  • 任务列表 - 查看所有任务的实时状态")
	fmt.Println("  • 执行历史 - 查看任务执行历史记录")
	fmt.Println("  • 统计分析 - 查看系统级统计信息")
	fmt.Println("\n按 Ctrl+C 停止程序")

	// 8. 等待中断信号
	<-ctx.Done()

	// 9. 优雅关闭
	fmt.Println("\n▸ 正在优雅关闭...")
	c.Stop()
	fmt.Println("=== 演示完成 ===")
}
