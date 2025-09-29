package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/darkit/cron"
)

func main() {
	fmt.Println("🚀 WithContext 生命周期管理示例")
	fmt.Println("==========================")

	// 示例1：信号驱动的优雅关闭
	fmt.Println("\n📡 创建信号监听的上下文...")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 创建绑定到信号上下文的调度器
	scheduler := cron.New(cron.WithContext(ctx))

	// 计数器
	counter1 := int64(0)
	counter2 := int64(0)
	counter3 := int64(0)

	// 添加几个不同频率的任务
	scheduler.Schedule("fast-task", "*/1 * * * * *", func(taskCtx context.Context) {
		count := atomic.AddInt64(&counter1, 1)
		fmt.Printf("⚡ 快速任务 #%d 执行\n", count)

		// 检查任务级别的取消信号
		select {
		case <-taskCtx.Done():
			fmt.Printf("  ↳ 快速任务 #%d 收到取消信号\n", count)
			return
		default:
			// 模拟工作
			time.Sleep(100 * time.Millisecond)
		}
	})

	scheduler.Schedule("medium-task", "*/3 * * * * *", func(taskCtx context.Context) {
		count := atomic.AddInt64(&counter2, 1)
		fmt.Printf("🔄 中等任务 #%d 执行\n", count)

		// 模拟长时间运行的任务，支持优雅取消
		for i := 0; i < 10; i++ {
			select {
			case <-taskCtx.Done():
				fmt.Printf("  ↳ 中等任务 #%d 在步骤 %d 被取消\n", count, i)
				return
			default:
				time.Sleep(200 * time.Millisecond)
				fmt.Printf("  ↳ 中等任务 #%d 步骤 %d 完成\n", count, i+1)
			}
		}
	})

	scheduler.Schedule("slow-task", "*/5 * * * * *", func(taskCtx context.Context) {
		count := atomic.AddInt64(&counter3, 1)
		fmt.Printf("🐌 慢速任务 #%d 开始执行\n", count)

		// 模拟可能很长的任务
		timer := time.NewTimer(4 * time.Second)
		defer timer.Stop()

		select {
		case <-timer.C:
			fmt.Printf("  ↳ 慢速任务 #%d 正常完成\n", count)
		case <-taskCtx.Done():
			fmt.Printf("  ↳ 慢速任务 #%d 被提前取消\n", count)
		}
	})

	// 启动调度器
	fmt.Printf("\n▶️  启动调度器...\n")
	err := scheduler.Start()
	if err != nil {
		fmt.Printf("❌ 启动失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 调度器已启动，按 Ctrl+C 或发送 SIGTERM 信号停止\n")
	fmt.Printf("💡 观察任务如何响应取消信号进行优雅关闭\n\n")

	// 等待信号
	<-ctx.Done()

	fmt.Printf("\n🛑 收到停止信号，调度器正在自动停止...\n")

	// 给一点时间让任务完成清理
	time.Sleep(500 * time.Millisecond)

	fmt.Printf("✅ 程序优雅退出\n")
	fmt.Printf("📊 最终统计:\n")
	fmt.Printf("  - 快速任务执行了 %d 次\n", atomic.LoadInt64(&counter1))
	fmt.Printf("  - 中等任务执行了 %d 次\n", atomic.LoadInt64(&counter2))
	fmt.Printf("  - 慢速任务执行了 %d 次\n", atomic.LoadInt64(&counter3))
}
