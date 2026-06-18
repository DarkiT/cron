package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/darkit/cron"
)

// CustomLogger 自定义日志实现
type CustomLogger struct {
	prefix string
}

func NewCustomLogger(prefix string) *CustomLogger {
	return &CustomLogger{prefix: prefix}
}

func (l *CustomLogger) Debugf(format string, args ...any) {
	log.Printf("[%s DEBUG] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *CustomLogger) Infof(format string, args ...any) {
	log.Printf("[%s INFO] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *CustomLogger) Warnf(format string, args ...any) {
	log.Printf("[%s WARN] %s", l.prefix, fmt.Sprintf(format, args...))
}

func (l *CustomLogger) Errorf(format string, args ...any) {
	log.Printf("[%s ERROR] %s", l.prefix, fmt.Sprintf(format, args...))
}

func main() {
	fmt.Println("🎨 自定义Logger示例")
	fmt.Println("演示如何使用自定义日志实现")
	fmt.Println("===========================")

	// 创建自定义logger
	customLogger := NewCustomLogger("CRON-APP")

	// 创建使用自定义logger的调度器
	c := cron.New(cron.WithLogger(customLogger))

	// 添加正常任务
	if err := c.Schedule("normal-task", "*/2 * * * * *", func(ctx context.Context) {
		fmt.Println("✅ 正常任务执行")
	}); err != nil {
		log.Fatalf("schedule normal-task failed: %v", err)
	}

	// 添加会panic的任务（测试错误日志，所有任务都内置panic保护）
	if err := c.Schedule("panic-task", "*/4 * * * * *", func(ctx context.Context) {
		fmt.Println("💥 即将panic...")
		panic("测试panic处理")
	}); err != nil {
		log.Fatalf("schedule panic-task failed: %v", err)
	}

	// 启动调度器
	if err := c.Start(); err != nil {
		log.Fatalf("start scheduler failed: %v", err)
	}
	defer c.Stop()

	fmt.Println("🚀 调度器已启动，观察自定义日志输出...")

	// 运行8秒
	time.Sleep(8 * time.Second)

	fmt.Println("📊 示例结束")
}
