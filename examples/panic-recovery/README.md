# Panic恢复示例

演示cron库强大的异常捕获和恢复功能，确保任务中的panic不会导致程序崩溃。

## 🛡️ 核心特性

### 1. **内置异常保护**
- 所有任务都自动具备panic捕获机制
- 自定义PanicHandler支持  
- 异常后继续执行，不影响其他任务

### 2. **灵活的处理策略**
- 默认panic处理器：打印错误信息
- 自定义panic处理器：实现PanicHandler接口
- 堆栈追踪：完整的错误上下文
- 恢复日志：清晰的异常恢复记录

## 🚀 运行示例

```bash
cd examples/panic-recovery
go run main.go
```

## 📋 示例包含

### 普通任务（内置保护）
```go
panicJob := &PanicJob{counter: &counter, shouldPanic: true}
scheduler.ScheduleJob("panic-job", "*/2 * * * * *", panicJob)
```

### 自动安全调度（内置保护）
```go
scheduler.Schedule("safe-task", "*/3 * * * * *", func(ctx context.Context) {
    // 即使这里panic，任务也会继续运行（内置panic保护）
    panic("不用担心，会被捕获！")
})
```

### 自定义异常处理器
```go
type CustomPanicHandler struct{}

func (h *CustomPanicHandler) HandlePanic(taskID string, panicValue interface{}, stack []byte) {
    log.Printf("🚨 任务 %s 发生panic: %v", taskID, panicValue)
    // 自定义恢复逻辑：发送告警、记录日志等
}

scheduler := cron.New(cron.WithPanicHandler(&CustomPanicHandler{}))
```

## ⭐ 观察重点

运行示例时注意观察：

1. **panic任务**会在第3次执行时panic，但不会停止调度
2. **安全panic任务**会在第2次执行时panic，然后继续运行  
3. **正常任务**不受影响，持续执行
4. **统计信息**正确记录成功/失败次数
5. **程序稳定**整个程序不会因为单个任务panic而崩溃

## 🔧 技术实现

### PanicHandler接口
```go
type PanicHandler interface {
    HandlePanic(taskID string, panicValue interface{}, stack []byte)
}
```

### SafeCall函数
```go
func SafeCall(taskID string, fn func(), handler PanicHandler) (recovered bool)
```

### RecoveryJob包装器
自动包装Job，添加panic恢复能力

## 🎯 最佳实践

1. **使用核心API**进行任务调度，所有任务都自动具备panic保护
2. **实现自定义**PanicHandler处理特定业务需求
3. **监控异常**通过统计信息跟踪任务健康状态
4. **优雅降级**panic后任务继续运行，保持系统稳定

这个示例完美展示了如何构建一个高可用的定时任务系统！🎉