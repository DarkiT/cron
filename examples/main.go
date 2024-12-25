package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/darkit/cron"
)

type simplePanicHandler struct{}

func (h *simplePanicHandler) Handle(jobName string, err interface{}) {
	log.Printf("捕获到panic [%s]: %v", jobName, err)
}

func main() {
	// 创建一个新的定时任务管理器，并配置panic处理
	scheduler := cron.New(
		cron.WithPanicHandler(&simplePanicHandler{}),
	)

	// 添加一个每5秒执行一次的任务
	err := scheduler.AddJob(cron.JobConfig{
		Name:     "print-time",
		Schedule: "*/5 * * * * *", // 每5秒执行一次
		Async:    true,            // 异步执行
	}, func() {
		fmt.Printf("当前时间: %v\n", time.Now().Format("2006-01-02 15:04:05"))
	})
	if err != nil {
		log.Fatal(err)
	}

	// 添加一个每分钟执行的带异常处理的任务
	err = scheduler.AddJob(cron.JobConfig{
		Name:     "risky-job",
		Schedule: "0 * * * * *", // 每分钟执行一次
		TryCatch: true,          // 启用异常捕获
	}, func() {
		panic("任务执行出错了！")
	})
	if err != nil {
		log.Fatal(err)
	}

	// 添加带超时和并发限制的任务
	err = scheduler.AddJob(cron.JobConfig{
		Name:          "concurrent-job",
		Schedule:      "*/1 * * * * *",
		Async:         true,
		MaxConcurrent: 2,
		Timeout:       time.Second * 5,
	}, func() {
		// 模拟耗时操作
		time.Sleep(time.Second * 3)
		fmt.Println("耗时任务执行完成")
	})
	if err != nil {
		log.Fatal(err)
	}

	// 启动调度器
	scheduler.Start()

	// 等待10秒后更新任务
	time.Sleep(10 * time.Second)

	// 更新打印时间的任务
	err = scheduler.UpdateJob(cron.JobConfig{
		Name:     "print-time",
		Schedule: "*/10 * * * * *", // 更新为每10秒执行一次
		Async:    true,
	}, func() {
		fmt.Printf("[执行时间:%d] 更新后的任务 - 当前时间: %v\n", time.Now().UnixNano(), time.Now().Format("2006-01-02 15:04:05"))
	})
	if err != nil {
		log.Printf("更新任务失败: %v", err)
	}

	// 等待信号优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 停止调度器
	scheduler.Stop()
}
