package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/darkit/cron"
	// 🎯 关键：通过空导入自动注册所有jobs包中的任务
	_ "github.com/darkit/cron/examples/jobs"
)

func main() {
	fmt.Println("🚀 真正的自动注册任务示例")
	fmt.Println("通过 _ \"github.com/darkit/cron/jobs\" 导入自动注册任务")
	fmt.Println("========================================")

	// 显示已自动注册的任务
	registered := cron.ListRegistered()
	fmt.Printf("📋 已自动注册的任务: %v\n", registered)
	fmt.Printf("📊 注册任务数量: %d\n", len(registered))

	// 创建调度器（使用自定义日志）
	scheduler := cron.New(cron.WithLogger(&CustomLogger{}))

	// 🎯 核心功能：一键调度所有已注册的任务
	fmt.Println("\n⚡ 正在调度所有已注册的任务...")
	err := scheduler.ScheduleRegistered()
	if err != nil {
		log.Fatalf("调度注册任务失败: %v", err)
	}

	// 手动添加一个普通任务作为对比
	counter := int64(0)
	err = scheduler.Schedule("manual-task", "*/2 * * * * *", func(ctx context.Context) {
		count := atomic.AddInt64(&counter, 1)
		fmt.Printf("[%s] 手动任务执行 (次数: %d)\n",
			time.Now().Format("15:04:05"), count)
	})
	if err != nil {
		log.Fatalf("添加手动任务失败: %v", err)
	}

	// 启动调度器
	err = scheduler.Start()
	if err != nil {
		log.Fatalf("启动调度器失败: %v", err)
	}

	// 显示任务信息
	fmt.Printf("\n📋 所有任务列表:\n")
	tasks := scheduler.List()
	for i, task := range tasks {
		nextRun, _ := scheduler.NextRun(task)
		fmt.Printf("  %d. %s (下次执行: %s)\n",
			i+1, task, nextRun.Format("15:04:05"))
	}

	// 监控任务统计
	go func() {
		time.Sleep(8 * time.Second)

		fmt.Println("\n📊 任务执行统计:")
		allStats := scheduler.GetAllStats()
		for id, stats := range allStats {
			fmt.Printf("- %s: 运行%d次，成功%d次，最后运行时间 %s\n",
				id, stats.RunCount, stats.SuccessCount,
				stats.LastRun.Format("15:04:05"))
		}
	}()

	// 等待信号
	fmt.Printf("\n▶️  调度器已启动，按 Ctrl+C 停止...\n\n")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 正在停止调度器...")
	scheduler.Stop()
	fmt.Println("✅ 调度器已停止")

	// 显示最终统计
	fmt.Printf("\n📊 最终统计: 手动任务执行 %d次\n", atomic.LoadInt64(&counter))
}

// CustomLogger 自定义日志
type CustomLogger struct{}

func (l *CustomLogger) Debugf(format string, args ...any) { log.Printf("[DEBUG] "+format, args...) }
func (l *CustomLogger) Infof(format string, args ...any)  { log.Printf("[INFO] "+format, args...) }
func (l *CustomLogger) Warnf(format string, args ...any)  { log.Printf("[WARN] "+format, args...) }
func (l *CustomLogger) Errorf(format string, args ...any) { log.Printf("[ERROR] "+format, args...) }
