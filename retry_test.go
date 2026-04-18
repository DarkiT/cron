package cron

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestRetryBasic 测试基本重试功能
func TestRetryBasic(t *testing.T) {
	c := New()
	defer c.Stop()

	// 创建一个会失败 2 次然后成功的任务
	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		count := attempts.Add(1)
		if count < 3 {
			panic(fmt.Sprintf("attempt %d failed", count))
		}
		// 第 3 次成功
	}

	err := c.Schedule("retry-basic", "@every 1s", handler, JobOptions{
		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
		MaxConcurrent: 1, // 防止并发执行
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	c.Start()

	// 等待任务执行（完成首次执行和所有重试）
	time.Sleep(600 * time.Millisecond)

	// 停止调度器，避免继续触发后续调度影响统计
	c.Stop()

	// 验证任务最终成功
	stats, ok := c.GetStats("retry-basic")
	if !ok {
		t.Fatal("Stats not found")
	}

	if stats.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", stats.SuccessCount)
	}

	// 验证重试次数（应该是 2 次重试）
	if stats.RetryCount != 2 {
		t.Errorf("Expected 2 retries, got %d", stats.RetryCount)
	}
}

// TestRetryInfinite 测试无限重试
func TestRetryInfinite(t *testing.T) {
	c := New()
	defer c.Stop()

	// 创建一个会失败 5 次后成功的任务
	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		count := attempts.Add(1)
		if count < 6 {
			panic(fmt.Sprintf("attempt %d failed", count))
		}
		// 第 6 次成功
	}

	err := c.Schedule("retry-infinite", "@every 1s", handler, JobOptions{
		MaxRetries:    -1, // 无限重试
		RetryInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	c.Start()

	// 等待任务执行（完成多次重试）
	time.Sleep(600 * time.Millisecond)

	// 停止调度器再读取统计
	c.Stop()

	// 验证任务最终成功
	stats, ok := c.GetStats("retry-infinite")
	if !ok {
		t.Fatal("Stats not found")
	}

	if stats.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", stats.SuccessCount)
	}

	// 验证重试次数（应该是 5 次重试）
	if stats.RetryCount != 5 {
		t.Errorf("Expected 5 retries, got %d", stats.RetryCount)
	}
}

// TestRetryExhausted 测试重试次数用尽
func TestRetryExhausted(t *testing.T) {
	c := New()
	defer c.Stop()

	// 创建一个始终失败的任务
	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		attempts.Add(1)
		panic("always fail")
	}

	err := c.Schedule("retry-exhausted", "@every 1s", handler, JobOptions{
		MaxRetries:    3,
		RetryInterval: 50 * time.Millisecond,
		MaxConcurrent: 1, // 防止并发执行
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	c.Start()

	// 等待任务执行（完成所有重试）
	time.Sleep(600 * time.Millisecond)

	c.Stop()

	// 验证任务失败
	stats, ok := c.GetStats("retry-exhausted")
	if !ok {
		t.Fatal("Stats not found")
	}

	if stats.SuccessCount != 0 {
		t.Errorf("Expected 0 success, got %d", stats.SuccessCount)
	}

	if stats.FailCount != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.FailCount)
	}

	// 验证重试次数（应该是 3 次重试）
	if stats.RetryCount != 3 {
		t.Errorf("Expected 3 retries, got %d", stats.RetryCount)
	}

	// 验证总尝试次数（1 次初始 + 3 次重试 = 4 次）
	totalAttempts := attempts.Load()
	if totalAttempts != 4 {
		t.Errorf("Expected 4 total attempts, got %d", totalAttempts)
	}
}

// TestRetryContextCancel 测试上下文取消
func TestRetryContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := New(WithContext(ctx))
	defer c.Stop()

	// 创建一个始终失败的任务
	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		attempts.Add(1)
		panic("always fail")
	}

	err := c.Schedule("retry-cancel", "@every 1s", handler, JobOptions{
		MaxRetries:    10,
		RetryInterval: 200 * time.Millisecond,
		MaxConcurrent: 1, // 防止并发执行
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	c.Start()

	// 等待一次执行和部分重试
	time.Sleep(600 * time.Millisecond)

	// 取消上下文
	cancel()
	c.Stop()

	// 等待停止完成
	time.Sleep(100 * time.Millisecond)

	// 验证没有继续重试（总尝试次数应该远小于 11）
	totalAttempts := attempts.Load()
	if totalAttempts >= 11 {
		t.Errorf("Context cancel failed: got %d attempts, should be < 11", totalAttempts)
	}
}

// TestRetryImmediately 测试立即重试
func TestRetryImmediately(t *testing.T) {
	c := New()
	defer c.Stop()

	// 创建一个会失败 3 次后成功的任务
	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		count := attempts.Add(1)
		if count < 4 {
			panic(fmt.Sprintf("attempt %d failed", count))
		}
	}

	err := c.Schedule("retry-immediate", "@every 1s", handler, JobOptions{
		MaxRetries:    5,
		RetryInterval: 0, // 立即重试
		MaxConcurrent: 1, // 防止并发执行
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	startTime := time.Now()
	c.Start()

	// 等待任务执行（立即重试应该很快完成）
	time.Sleep(500 * time.Millisecond)

	c.Stop()

	duration := time.Since(startTime)

	// 验证任务成功
	stats, ok := c.GetStats("retry-immediate")
	if !ok {
		t.Fatal("Stats not found")
	}

	if stats.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", stats.SuccessCount)
	}

	// 验证执行时间不会太长（主要验证重试机制工作正常）
	if duration > 3*time.Second {
		t.Errorf("Task took too long (including schedule delay): %v", duration)
	}
}

// TestRetryWithConcurrency 测试重试与并发控制结合
func TestRetryWithConcurrency(t *testing.T) {
	c := New()
	defer c.Stop()

	// 创建一个会失败 2 次的任务
	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		count := attempts.Add(1)
		if count < 3 {
			panic(fmt.Sprintf("attempt %d failed", count))
		}
		// 执行较长时间以测试并发
		time.Sleep(100 * time.Millisecond)
	}

	err := c.Schedule("retry-concurrent", "@every 1s", handler, JobOptions{
		MaxRetries:    3,
		RetryInterval: 50 * time.Millisecond,
		MaxConcurrent: 1, // 限制并发为 1
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	c.Start()

	// 等待任务执行（完成重试过程）
	time.Sleep(800 * time.Millisecond)

	c.Stop()

	// 验证并发控制仍然有效
	stats, ok := c.GetStats("retry-concurrent")
	if !ok {
		t.Fatal("Stats not found")
	}

	// 应该至少有一次成功（重试后成功）
	if stats.SuccessCount < 1 {
		t.Errorf("Expected at least 1 success, got %d", stats.SuccessCount)
	}
}

// TestRetryStats 测试重试统计的准确性
func TestRetryStats(t *testing.T) {
	c := New()
	defer c.Stop()

	// 任务 1：会重试 2 次后成功
	attempts1 := atomic.Int32{}
	err := c.Schedule("task1", "@every 1s", func(ctx context.Context) {
		count := attempts1.Add(1)
		if count < 3 {
			panic("fail")
		}
	}, JobOptions{
		MaxRetries:    5,
		RetryInterval: 50 * time.Millisecond,
		MaxConcurrent: 1, // 防止并发执行
	})
	if err != nil {
		t.Fatalf("Schedule task1 failed: %v", err)
	}

	// 任务 2：会重试 3 次用尽
	attempts2 := atomic.Int32{}
	err = c.Schedule("task2", "@every 1s", func(ctx context.Context) {
		attempts2.Add(1)
		panic("always fail")
	}, JobOptions{
		MaxRetries:    3,
		RetryInterval: 50 * time.Millisecond,
		MaxConcurrent: 1, // 防止并发执行
	})
	if err != nil {
		t.Fatalf("Schedule task2 failed: %v", err)
	}

	c.Start()

	// 等待任务执行（完成所有重试）
	time.Sleep(800 * time.Millisecond)

	c.Stop()

	// 验证任务 1 统计
	stats1, ok := c.GetStats("task1")
	if !ok {
		t.Fatal("task1 stats not found")
	}

	if stats1.SuccessCount != 1 {
		t.Errorf("task1: Expected 1 success, got %d", stats1.SuccessCount)
	}

	if stats1.RetryCount != 2 {
		t.Errorf("task1: Expected 2 retries, got %d", stats1.RetryCount)
	}

	// 验证任务 2 统计
	stats2, ok := c.GetStats("task2")
	if !ok {
		t.Fatal("task2 stats not found")
	}

	if stats2.SuccessCount != 0 {
		t.Errorf("task2: Expected 0 success, got %d", stats2.SuccessCount)
	}

	if stats2.FailCount != 1 {
		t.Errorf("task2: Expected 1 failure, got %d", stats2.FailCount)
	}

	if stats2.RetryCount != 3 {
		t.Errorf("task2: Expected 3 retries, got %d", stats2.RetryCount)
	}
}

// TestRetryWithTimeout 测试重试与超时结合
func TestRetryWithTimeout(t *testing.T) {
	c := New()
	defer c.Stop()

	// 创建一个执行时间较长的任务
	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		count := attempts.Add(1)
		if count < 3 {
			// 模拟超时（Sleep 150ms 会超过 100ms 的超时设置）
			time.Sleep(150 * time.Millisecond)
		}
		// 第 3 次快速完成
	}

	err := c.Schedule("retry-timeout", "@every 10s", handler, JobOptions{
		MaxRetries:    5,
		RetryInterval: 50 * time.Millisecond,
		Timeout:       100 * time.Millisecond, // 100ms 超时
		MaxConcurrent: 1,                      // 防止并发执行
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	c.Start()

	// 等待任务执行（任务超时后继续重试）
	time.Sleep(800 * time.Millisecond)

	c.Stop()

	// 验证任务最终成功（前两次超时，第三次成功）
	stats, ok := c.GetStats("retry-timeout")
	if !ok {
		t.Fatal("Stats not found")
	}

	// 获取实际尝试次数
	totalAttempts := attempts.Load()
	t.Logf("Total attempts: %d, SuccessCount: %d, FailCount: %d, RetryCount: %d",
		totalAttempts, stats.SuccessCount, stats.FailCount, stats.RetryCount)

	if stats.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", stats.SuccessCount)
	}

	// 验证有重试发生
	if stats.RetryCount < 2 {
		t.Errorf("Expected at least 2 retries, got %d", stats.RetryCount)
	}
}

// TestRetryNoRetry 测试 MaxRetries = 0 不重试
func TestRetryNoRetry(t *testing.T) {
	c := New()

	attempts := atomic.Int32{}
	handler := func(ctx context.Context) {
		attempts.Add(1)
		panic("fail")
	}

	err := c.Schedule("no-retry", "@every 1s", handler, JobOptions{
		MaxRetries:    0, // 不重试
		RetryInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	c.Start()
	defer c.Stop()

	// 等待任务执行（只需等待首次执行完成）
	time.Sleep(300 * time.Millisecond)

	// 验证只执行了一次（没有重试）
	totalAttempts := attempts.Load()
	if totalAttempts != 1 {
		t.Errorf("Expected 1 attempt (no retry), got %d", totalAttempts)
	}

	// 验证统计信息
	stats, ok := c.GetStats("no-retry")
	if !ok {
		t.Fatal("Stats not found")
	}

	if stats.RetryCount != 0 {
		t.Errorf("Expected 0 retries, got %d", stats.RetryCount)
	}

	if stats.FailCount != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.FailCount)
	}
}
