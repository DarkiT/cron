// Package cron 提供了一个简洁、高效的定时任务调度器，支持标准的 cron 表达式。
//
// 基本用法:
//
//	c := cron.New()
//	c.Schedule("job1", "*/5 * * * * *", func(ctx context.Context) {
//		fmt.Println("每5秒执行一次")
//	})
//	c.Start()
//	defer c.StopGracefully(5 * time.Second)
//
// 资源收口:
//
//	// Stop / StopGracefully 只停止调度，不关闭外部资源
//	c.Stop()
//
//	// Close 用于终局收口：停止调度并关闭托管资源（如 history recorder）
//	defer c.Close()
//
// 支持的功能:
//
// - 支持标准的cron表达式（5段/6段语法）
// - 自动处理5段语法（自动在秒位补0）
// - 支持同步和异步任务执行
// - 内置panic捕获机制
// - 支持动态添加、更新、删除任务
// - 支持一次性任务与有限次数任务 sugar API
// - 支持任务接口实现
// - 完善的日志记录
// - 优雅的停止机制
// - 支持Context上下文感知
// - 并发控制机制
// - 缓存解析的cron表达式
// - 可自定义日志接口和panic处理器
//
// 更多示例请查看 examples/ 目录
package cron
