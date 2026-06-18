package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/darkit/cron"
	"github.com/darkit/cron/history"
)

func main() {
	fmt.Println("=== Cron 历史记录功能演示 ===")

	// 1. 创建历史记录存储
	homeDir, _ := os.UserHomeDir()
	historyDir := filepath.Join(homeDir, ".cron_history")
	storage, err := history.NewFileStorage(historyDir)
	if err != nil {
		fmt.Printf("创建历史记录存储失败: %v\n", err)
		return
	}
	defer func() {
		if err := storage.Close(); err != nil {
			fmt.Printf("关闭历史记录存储失败: %v\n", err)
		}
	}()

	// 2. 创建历史记录器
	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		fmt.Printf("创建历史记录器失败: %v\n", err)
		return
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			fmt.Printf("关闭历史记录器失败: %v\n", err)
		}
	}()

	// 3. 创建启用历史记录的调度器
	c := cron.New(cron.WithHistoryRecorder(recorder))

	// 4. 添加示例任务
	fmt.Println("▸ 添加任务...")

	// 任务 1：每 3 秒执行的成功任务
	if err := c.Schedule("success-task", "@every 3s", func(ctx context.Context) {
		fmt.Println("  ✓ 成功任务执行")
	}); err != nil {
		fmt.Printf("添加 success-task 失败: %v\n", err)
		return
	}

	// 任务 2：每 5 秒执行的可能失败任务（带重试）
	attemptCount := 0
	if err := c.Schedule("retry-task", "@every 5s", func(ctx context.Context) {
		attemptCount++
		if attemptCount%3 == 1 {
			fmt.Println("  ✗ 重试任务失败（将重试）")
			panic("模拟失败")
		}
		fmt.Println("  ✓ 重试任务成功")
		attemptCount = 0
	}, cron.JobOptions{
		MaxRetries:    2,
		RetryInterval: 1 * time.Second,
	}); err != nil {
		fmt.Printf("添加 retry-task 失败: %v\n", err)
		return
	}

	// 任务 3：每 4 秒执行的必定失败任务
	if err := c.Schedule("fail-task", "@every 4s", func(ctx context.Context) {
		fmt.Println("  ✗ 失败任务执行失败")
		panic("必定失败")
	}, cron.JobOptions{
		MaxRetries: 0, // 不重试
	}); err != nil {
		fmt.Printf("添加 fail-task 失败: %v\n", err)
		return
	}

	// 5. 启动调度器
	if err := c.Start(); err != nil {
		fmt.Printf("启动调度器失败: %v\n", err)
		return
	}
	fmt.Println("▸ 调度器已启动")

	// 6. 运行一段时间收集历史记录
	fmt.Println("正在运行任务，收集历史记录...")
	time.Sleep(15 * time.Second)

	// 7. 停止调度器
	c.Stop()
	fmt.Println("\n▸ 调度器已停止")

	// 等待历史记录写入
	time.Sleep(500 * time.Millisecond)

	// 8. 查询和展示历史记录
	fmt.Println("=== 历史记录查询示例 ===")

	// 8.1 查询所有历史记录
	fmt.Println("1. 查询所有任务的历史记录:")
	allRecords, err := c.QueryHistory(history.RecordFilter{})
	if err != nil {
		fmt.Printf("   查询失败: %v\n", err)
	} else {
		fmt.Printf("   总记录数: %d\n", len(allRecords))
		for i, record := range allRecords {
			if i >= 5 {
				fmt.Printf("   ... 还有 %d 条记录\n", len(allRecords)-5)
				break
			}
			status := "✓ 成功"
			if !record.Success {
				status = "✗ 失败"
			}
			fmt.Printf("   - %s | %s | 耗时: %v | 重试: %d次\n",
				record.TaskID, status, record.Duration, record.RetryCount)
		}
	}

	// 8.2 查询特定任务的历史
	fmt.Println("\n2. 查询 retry-task 的历史记录:")
	retryRecords, err := c.QueryHistory(history.RecordFilter{
		TaskID: "retry-task",
	})
	if err != nil {
		fmt.Printf("   查询失败: %v\n", err)
	} else {
		fmt.Printf("   记录数: %d\n", len(retryRecords))
		for _, record := range retryRecords {
			status := "✓ 成功"
			if !record.Success {
				status = "✗ 失败"
			}
			fmt.Printf("   - %s | 耗时: %v | 重试: %d次\n",
				status, record.Duration, record.RetryCount)
		}
	}

	// 8.3 仅查询成功的记录
	fmt.Println("\n3. 查询所有成功的记录:")
	successRecords, err := c.QueryHistory(history.RecordFilter{
		SuccessOnly: true,
	})
	if err != nil {
		fmt.Printf("   查询失败: %v\n", err)
	} else {
		fmt.Printf("   成功记录数: %d\n", len(successRecords))
	}

	// 8.4 仅查询失败的记录
	fmt.Println("\n4. 查询所有失败的记录:")
	failedRecords, err := c.QueryHistory(history.RecordFilter{
		FailedOnly: true,
	})
	if err != nil {
		fmt.Printf("   查询失败: %v\n", err)
	} else {
		fmt.Printf("   失败记录数: %d\n", len(failedRecords))
		for i, record := range failedRecords {
			if i >= 3 {
				fmt.Printf("   ... 还有 %d 条失败记录\n", len(failedRecords)-3)
				break
			}
			fmt.Printf("   - %s | 错误: %s | 重试: %d次\n",
				record.TaskID, record.Error, record.RetryCount)
		}
	}

	// 8.5 统计记录数量
	fmt.Println("\n5. 统计各任务的记录数量:")
	for _, taskID := range []string{"success-task", "retry-task", "fail-task"} {
		count, err := c.CountHistory(history.RecordFilter{
			TaskID: taskID,
		})
		if err != nil {
			fmt.Printf("   %s: 统计失败 (%v)\n", taskID, err)
		} else {
			fmt.Printf("   %s: %d 条记录\n", taskID, count)
		}
	}

	// 8.6 时间范围查询
	fmt.Println("\n6. 查询最近 10 秒的记录:")
	now := time.Now()
	tenSecondsAgo := now.Add(-10 * time.Second)
	recentRecords, err := c.QueryHistory(history.RecordFilter{
		StartTime: &tenSecondsAgo,
		EndTime:   &now,
	})
	if err != nil {
		fmt.Printf("   查询失败: %v\n", err)
	} else {
		fmt.Printf("   最近 10 秒内的记录数: %d\n", len(recentRecords))
	}

	// 9. 清理旧记录演示
	fmt.Println("\n7. 清理历史记录演示:")
	fmt.Println("   注意：本示例不会实际清理，仅演示 API 用法")
	fmt.Println("   清理 7 天前的记录:")
	fmt.Println("   deleted, err := c.CleanupHistory(time.Now().Add(-7 * 24 * time.Hour))")

	// 10. 历史记录存储位置
	fmt.Printf("\n历史记录存储位置: %s\n", historyDir)
	fmt.Println("文件结构: <任务ID>/<日期>.json")

	fmt.Println("\n=== 演示完成 ===")
}
