# 极简示例

展示cron库最基本的用法，包括：

## 功能特性

- ✨ **函数调度** - 直接使用`Schedule()`方法调度函数
- 🎯 **Job接口** - 使用`ScheduleJob()`方法调度实现Job接口的任务  
- ⚙️ **配置选项** - 演示超时、异步、并发控制等选项
- 📊 **监控统计** - 展示任务执行统计和状态监控
- 🎨 **自定义日志** - 使用自定义Logger接口

## 运行示例

```bash
cd examples/minimal
go run main.go
```

## 核心API

```go
// 最简单的函数调度
scheduler.Schedule("task-id", "*/2 * * * * *", func(ctx context.Context) {
    // 任务执行逻辑
})

// Job接口调度，支持选项配置  
scheduler.ScheduleJob("job-id", "*/3 * * * * *", job, cron.JobOptions{
    Timeout: 5 * time.Second,
    Async:   true,
})
```

这个示例完美展示了新API的简洁性 - 只需要2个核心方法就能覆盖所有使用场景！