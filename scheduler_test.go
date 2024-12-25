package cron

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_Register(t *testing.T) {
	scheduler := NewCronScheduler()

	// 测试注册任务
	job, err := NewJobModel("*/1 * * * * *", func() {})
	if err != nil {
		t.Fatal(err)
	}

	err = scheduler.Register("test", job)
	if err != nil {
		t.Fatal(err)
	}

	// 测试重复注册
	err = scheduler.Register("test", job)
	if !errors.Is(err, ErrAlreadyRegister) {
		t.Fatalf("期望错误 %v, 得到 %v", ErrAlreadyRegister, err)
	}
}

func TestScheduler_DynamicRegister(t *testing.T) {
	scheduler := NewCronScheduler()
	executed := make(chan bool, 1)

	job, _ := NewJobModel("*/1 * * * * *", func() {
		executed <- true
	})

	scheduler.Start()

	// 等待调度器完全启动
	time.Sleep(100 * time.Millisecond)

	err := scheduler.DynamicRegister("test", job)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-executed:
		// 任务成功执行
	case <-time.After(2 * time.Second):
		t.Fatal("动态注册的任务没有执行")
	}

	scheduler.Stop()
}

func TestScheduler_StopService(t *testing.T) {
	scheduler := NewCronScheduler()
	var count int32

	job, _ := NewJobModel("*/1 * * * * *", func() {
		atomic.AddInt32(&count, 1)
	})

	scheduler.Register("test", job)
	scheduler.Start()

	// 等待任务执行几次
	time.Sleep(2 * time.Second)
	scheduler.StopService("test")
	currentCount := atomic.LoadInt32(&count)

	// 再等待一段时间，确认任务已停止
	time.Sleep(2 * time.Second)
	finalCount := atomic.LoadInt32(&count)

	if finalCount != currentCount {
		t.Fatal("停止服务后任务仍在执行")
	}
}
