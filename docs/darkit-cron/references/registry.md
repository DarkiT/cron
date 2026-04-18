# Registry Reference

当用户问题落在批量注册任务、`RegisteredJob` 设计、全局注册表副作用、registry 调度方式时，再读本文件。

## Preferred Shape

优先使用独立实例注册表，不要默认依赖全局状态：

```go
reg := cron.NewJobRegistry()
reg.SafeRegister(&SyncJob{})
reg.SafeRegister(&CleanupJob{})

c := cron.New()
if err := c.ScheduleFromRegistry(reg); err != nil {
    return err
}
```

这样做的好处：

- 避免 package-level global state
- 更适合测试隔离
- 多个模块可以各自持有自己的 registry

## `RegisteredJob` Contract

要进入 registry，任务需要实现：

```go
type RegisteredJob interface {
    Job
    Name() string
    Schedule() string
}
```

也就是说：

- 既要有 `Run(ctx) error`
- 也要有稳定的任务名
- 还要自带默认 schedule

## Registration Semantics

实例注册表的公开入口是 `SafeRegister(job)`。

注册时会校验：

- `job` 不能为 `nil`
- `Name()` 必须是合法 task ID
- `Schedule()` 必须是合法 schedule
- 名称不能重复

注意：

- `SafeRegister` 是“安全注册”风格，失败时主要靠日志，不向上抛 panic
- 如果你在做框架封装，最好在注册前就保证 `Name()` 与 `Schedule()` 可预测且稳定

## Scheduling From Registry

批量调度入口：

- `ScheduleFromRegistry(reg, ...opts)`：推荐
- `ScheduleRegistered(...opts)`：兼容旧代码，依赖全局注册表

语义要点：

- 这两个入口都最多只接受一个默认 `JobOptions`
- 该默认 `JobOptions` 会应用到批量加入的每个任务
- 真正的任务 ID 仍以注册时的 `Name()` 为准

## Global Registry Compatibility Layer

只有在兼容旧代码或 `init()` 自动注册模式下，才考虑这些全局 API：

- `RegisterJob`
- `SafeRegisterJob`
- `GetRegisteredJobs`
- `ListRegistered`
- `GetRegisteredJob`
- `ScheduleRegistered`

不推荐把新系统建立在全局 registry 上，因为它会引入：

- 隐式共享状态
- 测试顺序耦合
- 重复注册和清理困难

## Verification Hints

如果当前工作区带有 upstream 源码，优先核对：

- `registry.go` 中 `NewJobRegistry`、`SafeRegister`、`ScheduleFromRegistry`
- `normalizeTaskID` 与 `normalizeScheduleSpec` 对名称和 schedule 的约束
