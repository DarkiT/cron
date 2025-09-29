package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithContextBasic 测试 WithContext 基本功能
func TestWithContextBasic(t *testing.T) {
	// 创建一个5秒后取消的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 使用自定义上下文创建调度器
	c := New(WithContext(ctx))

	counter := int64(0)
	err := c.Schedule("context-test", "*/10 * * * * *", func(taskCtx context.Context) {
		atomic.AddInt64(&counter, 1)
		// 验证传入的上下文会响应取消
		select {
		case <-time.After(200 * time.Millisecond): // 超过上下文超时时间
			t.Error("Task should have been cancelled")
		case <-taskCtx.Done():
			// 正常，任务收到取消信号
		}
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// 等待上下文超时
	<-ctx.Done()

	// 给一点时间让调度器处理取消
	time.Sleep(50 * time.Millisecond)

	// 调度器应该已经停止
	if c.IsRunning() {
		t.Error("Scheduler should have stopped when context was cancelled")
	}

	c.Stop() // 确保清理
}

// TestWithContextNil 测试传入 nil 上下文的情况
func TestWithContextNil(t *testing.T) {
	c := New(WithContext(nil)) // 传入 nil

	counter := int64(0)
	err := c.Schedule("nil-context-test", "*/10 * * * * *", func(ctx context.Context) {
		atomic.AddInt64(&counter, 1)
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer c.Stop()

	// 等待一小段时间确保任务可以正常执行
	time.Sleep(20 * time.Millisecond)

	// 调度器应该仍在运行（因为使用了 Background context）
	if !c.IsRunning() {
		t.Error("Scheduler should be running with nil context (defaults to Background)")
	}
}

// TestWithContextCascadingCancel 测试级联取消
func TestWithContextCascadingCancel(t *testing.T) {
	// 父上下文
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	// 子上下文（调度器使用）
	childCtx, childCancel := context.WithCancel(parentCtx)
	defer childCancel()

	c := New(WithContext(childCtx))

	taskExecuted := int64(0)
	taskCancelled := int64(0)

	err := c.Schedule("cascade-test", "*/10 * * * * *", func(taskCtx context.Context) {
		atomic.AddInt64(&taskExecuted, 1)
		select {
		case <-time.After(200 * time.Millisecond):
			// 不应该到这里
		case <-taskCtx.Done():
			atomic.AddInt64(&taskCancelled, 1)
		}
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// 等一点时间
	time.Sleep(20 * time.Millisecond)

	// 取消父上下文，应该级联取消子上下文和调度器
	parentCancel()

	// 等待级联取消生效
	time.Sleep(50 * time.Millisecond)

	// 验证调度器已停止
	if c.IsRunning() {
		t.Error("Scheduler should have stopped when parent context was cancelled")
	}

	c.Stop() // 确保清理
}

// TestDefaultContextBehavior 测试默认行为（不使用 WithContext）
func TestDefaultContextBehavior(t *testing.T) {
	// 不使用 WithContext，应该使用默认的 Background context
	c := New()

	counter := int64(0)
	err := c.Schedule("default-test", "*/10 * * * * *", func(ctx context.Context) {
		atomic.AddInt64(&counter, 1)
	})
	if err != nil {
		t.Fatalf("Failed to schedule task: %v", err)
	}

	err = c.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer c.Stop()

	time.Sleep(20 * time.Millisecond)

	// 应该正常运行
	if !c.IsRunning() {
		t.Error("Default scheduler should be running")
	}
}
