# WithContext 生命周期管理示例

演示如何使用 `WithContext` 选项实现调度器与应用生命周期的绑定和优雅关闭。

## 🎯 核心特性

- **🔗 生命周期绑定** - 调度器自动响应上层应用的生命周期管理
- **🛑 优雅关闭** - 收到取消信号时，所有任务都能感知并优雅退出
- **📡 信号集成** - 与系统信号（SIGINT、SIGTERM）无缝集成
- **⛓️ 级联取消** - 支持多层次的上下文取消传播

## 运行示例

```bash
cd examples/context-lifecycle
go run main.go
```

运行后：
1. 程序会启动三个不同频率的任务
2. 按 `Ctrl+C` 发送中断信号
3. 观察所有任务如何优雅地响应取消信号并退出

## 🚀 实际应用场景

### 1. Web服务器集成
```go
func main() {
    // 创建可取消的上下文
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()
    
    // 调度器绑定到应用生命周期
    scheduler := cron.New(cron.WithContext(ctx))
    
    // 启动Web服务器和调度器
    server := &http.Server{Addr: ":8080"}
    scheduler.Start()
    
    go func() {
        server.ListenAndServe()
    }()
    
    // 等待停止信号
    <-ctx.Done()
    
    // 调度器会自动停止，无需手动调用
    server.Shutdown(context.Background())
}
```

### 2. 微服务中的定时任务
```go
func (s *Service) StartBackgroundTasks(ctx context.Context) {
    // 任务的生命周期与服务实例绑定
    taskScheduler := cron.New(cron.WithContext(ctx))
    
    taskScheduler.Schedule("health-check", "@every 30s", func(taskCtx context.Context) {
        // 执行健康检查
        // 服务停止时任务自动停止
    })
    
    taskScheduler.Start()
    // 服务关闭时，ctx 被取消，任务自动停止
}
```

### 3. 测试场景中的精确控制
```go
func TestWithTimeout(t *testing.T) {
    // 创建5秒超时的上下文
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // 调度器将在5秒后自动停止
    scheduler := cron.New(cron.WithContext(ctx))
    
    // 无需手动管理调度器生命周期
    scheduler.Schedule("test-task", "@every 1s", testHandler)
    scheduler.Start()
    
    // 测试逻辑...
    // 5秒后调度器自动停止
}
```

## ⭐ 设计优势

1. **零代码侵入** - 现有代码无需修改，完全向后兼容
2. **符合Go习惯** - 遵循标准的 context 使用模式
3. **自动化管理** - 减少手动生命周期管理的复杂性
4. **级联响应** - 支持复杂的应用架构中的层次化取消

## 🔍 关键观察点

运行示例时注意：
1. 不同频率任务的执行模式
2. 按 Ctrl+C 后任务如何优雅停止
3. 长时间运行任务的中断和清理行为
4. 调度器的自动停止响应