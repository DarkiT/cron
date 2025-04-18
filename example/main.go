package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/darkit/cron"
)

// 自定义panic处理器
type MyPanicHandler struct{}

func (h *MyPanicHandler) Handle(jobName string, err interface{}) {
	fmt.Printf("Job [%s] panicked: %v\n", jobName, err)
}

// 实现普通CronJob接口的任务
type SimpleJob struct {
	name string
	rule string
}

func (j *SimpleJob) Name() string {
	return j.name
}

func (j *SimpleJob) Rule() string {
	return j.rule
}

func (j *SimpleJob) Execute() {
	fmt.Printf("Executing simple job: %s, time: %v\n", j.name, time.Now().Format("15:04:05"))
}

// 实现支持上下文的CronJob接口
type ContextAwareJob struct {
	name string
	rule string
}

func (j *ContextAwareJob) Name() string {
	return j.name
}

func (j *ContextAwareJob) Rule() string {
	return j.rule
}

// 实现普通Execute方法 (CronJob接口)
func (j *ContextAwareJob) Execute() {
	fmt.Printf("Executing non-context job: %s\n", j.name)
}

// 实现支持上下文的方法 (CronJobWithContext接口)
func (j *ContextAwareJob) ExecuteWithContext(ctx context.Context) {
	fmt.Printf("Starting context-aware job: %s\n", j.name)

	select {
	case <-time.After(2 * time.Second):
		fmt.Printf("Job %s completed normally\n", j.name)
	case <-ctx.Done():
		fmt.Printf("Job %s was cancelled: %v\n", j.name, ctx.Err())
	}
}

// 自定义日志
type MyLogger struct{}

func (l *MyLogger) Debug(msg string) {
	fmt.Printf("[DEBUG] %s\n", msg)
}

func (l *MyLogger) Info(msg string) {
	fmt.Printf("[INFO] %s\n", msg)
}

func (l *MyLogger) Warn(msg string) {
	fmt.Printf("[WARN] %s\n", msg)
}

func (l *MyLogger) Error(msg string) {
	fmt.Printf("[ERROR] %s\n", msg)
}

func main() {
	// 配置标准日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// 创建带panic处理器和自定义日志的定时任务调度器
	scheduler := cron.New(
		cron.WithPanicHandler(&MyPanicHandler{}),
		cron.WithLogger(&MyLogger{}),
	)

	// 1. 添加简单任务
	_ = scheduler.AddFunc("simple-task", "*/5 * * * * *", func() {
		fmt.Printf("Executing simple function task, time: %v\n", time.Now().Format("15:04:05"))
	})

	// 2. 添加带配置的任务
	_ = scheduler.AddJob(cron.JobConfig{
		Name:          "configured-task",
		Schedule:      "*/10 * * * * *",
		Async:         true,
		TryCatch:      true,
		MaxConcurrent: 2,
		Timeout:       5 * time.Second,
	}, func() {
		fmt.Printf("Executing configured task, time: %v\n", time.Now().Format("15:04:05"))
		time.Sleep(3 * time.Second)
	})

	// 3. 添加会触发panic的任务
	_ = scheduler.AddFunc("panic-task", "*/30 * * * * *", func() {
		fmt.Println("This task will panic")
		panic("intentional panic")
	})

	// 4. 添加实现CronJob接口的任务
	simpleJob := &SimpleJob{
		name: "interface-job",
		rule: "*/15 * * * * *",
	}
	_ = scheduler.AddJobInterface(simpleJob)

	// 5. 添加支持Context的任务
	contextJob := &ContextAwareJob{
		name: "context-job",
		rule: "*/12 * * * * *",
	}
	_ = scheduler.AddJobInterface(contextJob)

	// 6. 添加会超时的任务
	_ = scheduler.AddJob(cron.JobConfig{
		Name:     "timeout-task",
		Schedule: "*/20 * * * * *",
		Timeout:  2 * time.Second,
	}, func() {
		fmt.Println("Starting timeout task, will run for 5 seconds")
		time.Sleep(5 * time.Second)
		fmt.Println("Timeout task finished (may not show)")
	})

	// 7. 添加使用WithContextFunc的任务
	jobModel, _ := cron.NewJobModel("*/8 * * * * *", nil,
		cron.WithContextFunc(func(ctx context.Context) {
			fmt.Println("Starting WithContextFunc task")
			select {
			case <-time.After(3 * time.Second):
				fmt.Println("WithContextFunc task completed normally")
			case <-ctx.Done():
				fmt.Println("WithContextFunc task was cancelled:", ctx.Err())
			}
		}),
		cron.WithTimeout(5*time.Second),
	)
	_ = scheduler.Register("context-func-job", jobModel)

	// 启动调度器
	scheduler.Start()
	fmt.Println("Scheduler started, press Ctrl+C to stop")

	// 打印所有任务的下一次执行时间
	for _, jobName := range scheduler.ListJobs() {
		nextTime, _ := scheduler.NextRuntime(jobName)
		fmt.Printf("Job [%s] next execution time: %v\n", jobName, nextTime.Format("15:04:05"))
	}

	// 等待信号退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Stopping scheduler...")
	scheduler.Stop()
	fmt.Println("Scheduler stopped, program exiting")
}
