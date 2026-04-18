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

// BackupJob 自动备份任务
type BackupJob struct {
	name    string
	counter *int64
}

var backupCounter = int64(0)

var reg = cron.NewJobRegistry()

func init() {
	// 在包初始化时注册到实例化注册表，避免全局共享
	job := &BackupJob{
		name:    "自动备份",
		counter: &backupCounter,
	}
	reg.SafeRegister(job)
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
	case <-time.After(500 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CleanupJob 自动清理任务
type CleanupJob struct {
	name    string
	counter *int64
}

var cleanupCounter = int64(0)

func init() {
	// 在包初始化时注册到实例化注册表
	job := &CleanupJob{
		name:    "自动清理",
		counter: &cleanupCounter,
	}
	reg.SafeRegister(job)
}

func (j *CleanupJob) Name() string {
	return "auto-cleanup"
}

func (j *CleanupJob) Schedule() string {
	return "*/5 * * * * *" // 每5秒执行一次
}

func (j *CleanupJob) Run(ctx context.Context) error {
	count := atomic.AddInt64(j.counter, 1)
	fmt.Printf("[%s] %s 执行清理操作 (次数: %d)\n",
		time.Now().Format("15:04:05"), j.name, count)

	// 模拟清理操作
	select {
	case <-time.After(200 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	fmt.Println("🚀 自动注册任务示例")
	fmt.Println("通过 _ \"package/jobs\" 导入自动注册任务")
	fmt.Println("==============================")

	// 显示已注册的任务
	registered := reg.List()
	fmt.Printf("📋 已自动注册的任务: %v\n", registered)

	// 创建调度器
	scheduler := cron.New()

	// 一键调度所有已注册的任务
	err := scheduler.ScheduleFromRegistry(reg, cron.JobOptions{
		Timeout: 5 * time.Second,
		Async:   true,
	})
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

	// 监控任务统计
	go func() {
		time.Sleep(8 * time.Second)

		fmt.Println("\n📊 任务统计:")
		allStats := scheduler.GetAllStats()
		for id, stats := range allStats {
			fmt.Printf("- %s: 运行%d次，成功%d次，最后运行时间 %s\n",
				id, stats.RunCount, stats.SuccessCount,
				stats.LastRun.Format("15:04:05"))
		}

		fmt.Println("\n📋 任务列表:")
		tasks := scheduler.List()
		for i, task := range tasks {
			nextRun, _ := scheduler.NextRun(task)
			fmt.Printf("  %d. %s (下次执行: %s)\n",
				i+1, task, nextRun.Format("15:04:05"))
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
}
