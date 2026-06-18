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

// CustomPanicHandler 自定义panic处理器
type CustomPanicHandler struct{}

func (h *CustomPanicHandler) HandlePanic(taskID string, panicValue any, stack []byte) {
	log.Printf("🚨 PANIC CAUGHT in task %s: %v", taskID, panicValue)
	log.Printf("📍 Stack trace (first 500 chars): %.500s", string(stack))
	fmt.Printf("✅ Task %s has been safely recovered and will continue running\n", taskID)
}

// PanicJob 会产生panic的任务
type PanicJob struct {
	counter     *int64
	shouldPanic bool
}

func (j *PanicJob) Name() string {
	return "panic-job"
}

func (j *PanicJob) Run(ctx context.Context) error {
	count := atomic.AddInt64(j.counter, 1)
	fmt.Printf("[%s] PanicJob 执行中... (次数: %d)\n",
		time.Now().Format("15:04:05"), count)

	// 第3次执行时故意panic
	if count == 3 && j.shouldPanic {
		panic(fmt.Sprintf("故意制造的panic! 执行次数: %d", count))
	}

	return nil
}

func main() {
	fmt.Println("🛡️  异常捕获和恢复示例")
	fmt.Println("演示如何优雅地处理任务中的panic")
	fmt.Println("=====================================")

	// 创建带自定义panic处理器的调度器
	scheduler := cron.New(cron.WithPanicHandler(&CustomPanicHandler{}))

	counter1 := int64(0)
	counter2 := int64(0)
	counter3 := int64(0)

	// 1. 普通任务（可能panic）- 使用内置的panic保护
	panicJob := &PanicJob{counter: &counter1, shouldPanic: true}
	err := scheduler.ScheduleJob("panic-job", "*/2 * * * * *", panicJob)
	if err != nil {
		log.Fatalf("调度panic任务失败: %v", err)
	}

	// 2. 带恢复功能的任务（所有任务都内置panic保护）
	if err := scheduler.Schedule("safe-panic-task", "*/3 * * * * *", func(ctx context.Context) {
		count := atomic.AddInt64(&counter2, 1)
		fmt.Printf("[%s] SafePanicTask 执行中... (次数: %d)\n",
			time.Now().Format("15:04:05"), count)

		// 第2次执行时panic，但会被捕获
		if count == 2 {
			panic(fmt.Sprintf("SafePanicTask 中的panic! 次数: %d", count))
		}
	}); err != nil {
		log.Fatalf("调度 safe-panic-task 失败: %v", err)
	}

	// 3. 永不panic的任务作为对比
	if err := scheduler.Schedule("normal-task", "*/1 * * * * *", func(ctx context.Context) {
		count := atomic.AddInt64(&counter3, 1)
		fmt.Printf("[%s] 正常任务执行 (次数: %d)\n",
			time.Now().Format("15:04:05"), count)
	}); err != nil {
		log.Fatalf("调度 normal-task 失败: %v", err)
	}

	// 启动调度器
	err = scheduler.Start()
	if err != nil {
		log.Fatalf("启动调度器失败: %v", err)
	}

	// 监控统计
	go func() {
		time.Sleep(10 * time.Second)

		fmt.Println("\n📊 执行统计:")
		allStats := scheduler.GetAllStats()
		for id, stats := range allStats {
			fmt.Printf("- %s: 运行%d次，成功%d次\n",
				id, stats.RunCount, stats.SuccessCount)
		}
	}()

	// 等待信号
	fmt.Printf("\n▶️  调度器已启动，观察panic处理...\n")
	fmt.Printf("⚠️  某些任务会故意panic，但程序不会崩溃！\n\n")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 正在停止调度器...")
	scheduler.Stop()
	fmt.Println("✅ 调度器已安全停止")

	// 显示最终统计
	fmt.Printf("\n📊 最终执行统计:\n")
	fmt.Printf("- Panic任务: %d次\n", atomic.LoadInt64(&counter1))
	fmt.Printf("- 安全Panic任务: %d次\n", atomic.LoadInt64(&counter2))
	fmt.Printf("- 正常任务: %d次\n", atomic.LoadInt64(&counter3))
}
