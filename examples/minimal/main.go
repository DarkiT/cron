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
)

// SimpleJob 实现Job接口的简单任务
type SimpleJob struct {
	name    string
	counter *int64
}

func (j *SimpleJob) Name() string {
	return j.name
}

func (j *SimpleJob) Run(ctx context.Context) error {
	count := atomic.AddInt64(j.counter, 1)
	fmt.Printf("[%s] %s executed (count: %d)\n",
		time.Now().Format("15:04:05"), j.name, count)
	return nil
}

// CustomLogger 自定义日志
type CustomLogger struct{}

func (l *CustomLogger) Debugf(format string, args ...any) { log.Printf("[DEBUG] "+format, args...) }
func (l *CustomLogger) Infof(format string, args ...any)  { log.Printf("[INFO] "+format, args...) }
func (l *CustomLogger) Warnf(format string, args ...any)  { log.Printf("[WARN] "+format, args...) }
func (l *CustomLogger) Errorf(format string, args ...any) { log.Printf("[ERROR] "+format, args...) }

func main() {
	fmt.Println("🚀 极简Cron调度器示例")
	fmt.Println("新一代API设计，零学习成本")
	fmt.Println("===============================")

	// 创建调度器
	scheduler := cron.New(cron.WithLogger(&CustomLogger{}))

	// 计数器
	counter1 := int64(0)
	counter2 := int64(0)

	// 方式1：最简单的函数调度
	err := scheduler.Schedule("simple-task", "*/2 * * * * *", func(ctx context.Context) {
		count := atomic.AddInt64(&counter1, 1)
		fmt.Printf("[%s] 简单任务执行 (次数: %d)\n",
			time.Now().Format("15:04:05"), count)
	})
	if err != nil {
		log.Fatalf("添加简单任务失败: %v", err)
	}

	// 方式2：使用Job接口的任务
	job := &SimpleJob{name: "接口任务", counter: &counter2}
	err = scheduler.ScheduleJob("interface-task", "*/3 * * * * *", job, cron.JobOptions{
		Timeout: 5 * time.Second,
		Async:   true,
	})
	if err != nil {
		log.Fatalf("添加接口任务失败: %v", err)
	}

	// 方式3：🌟 新API - 自动使用Job的Name()方法
	err = scheduler.ScheduleJobByName("*/4 * * * * *", &SimpleJob{
		name:    "优雅任务",
		counter: &counter2,
	}, cron.JobOptions{
		Async:         true,
		MaxConcurrent: 2,
		Timeout:       10 * time.Second,
	})
	if err != nil {
		log.Fatalf("添加优雅任务失败: %v", err)
	}

	// 启动调度器
	err = scheduler.Start()
	if err != nil {
		log.Fatalf("启动调度器失败: %v", err)
	}

	// 监控任务统计
	go func() {
		time.Sleep(10 * time.Second)

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

	// 显示最终统计
	fmt.Printf("\n📊 最终统计:\n")
	fmt.Printf("- 简单任务执行: %d次\n", atomic.LoadInt64(&counter1))
	fmt.Printf("- 接口任务执行: %d次\n", atomic.LoadInt64(&counter2))
}
