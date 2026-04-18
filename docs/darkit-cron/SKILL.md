---
name: darkit-cron
description: Use this skill when working with github.com/darkit/cron: building or refactoring Go scheduled jobs, choosing between function and Job APIs, configuring JobOptions (timeout/retry/async/concurrency/misfire/fail-fuse/startAt/maxRuns), using one-shot or finite-run helpers, runtime control APIs (RunNow/Update/Pause/Resume/StopGracefully/Close), history recording, registry-based job composition, or cron-oriented troubleshooting.
---

# darkit-cron

面向 `github.com/darkit/cron` 的代码级 Skill。它是给 AI 的最短上手路径：先选对 API，再按真实语义做修改，最后去对的边界测试验证。

## Use When

- 在 Go 项目中接入或修改 `github.com/darkit/cron`
- 新增、重构、排查 scheduled job
- 需要判断该用函数任务、`Job` 任务还是 registry 模式
- 需要配置 `JobOptions`
- 需要一次执行、指定首次执行时间、有限次数执行
- 需要运行时控制：`RunNow`、`Update`、`Pause`、`Resume`、`Remove`
- 需要 history、event hook、stats、lifecycle 语义

## Truth Source

这份 Skill 基于 upstream 代码与测试提炼，不依赖 `docs/` 下旧文档。

如果当前工作区包含 `darkit/cron` 源码，以这些文件为真理源：

- `cron.go`：公开 API、helper API、lifecycle
- `scheduler.go`：真实调度语义、`StartAt`、`MaxRuns`、`RunNow`、misfire、fail-fuse
- `registry.go`：registry 模式
- `history/recorder.go`：history recorder 生命周期与存储语义
- `api_boundary_test.go`：边界行为
- `history_test.go`：history 与 stop/start 行为

## Minimal Mental Model

先记住这 8 件事：

1. `Cron` 是公开入口，真正的调度语义在内部 scheduler。
2. 任务有两种主写法：
   - 函数：`Schedule`
   - `Job`：`ScheduleJob` / `ScheduleJobByName`
3. 一次任务和有限次任务本质上都是 `StartAt + MaxRuns`，但优先用 helper API，不要手搓。
4. `RunNow(id)` 是额外触发一次，不会改写正常计划，也不会消耗计划内 `MaxRuns`。
5. `StartAt` 对 `@every` 也生效，而且会作为时间锚点。
6. 计划次数耗尽或计划已过期时，有限次任务会自动移除。
7. `Stop()` / `StopGracefully()` 只停调度；`Close()` 才是终局收口，关闭后实例不可复用。
8. history 和 registry 是可选层，不是调度核心。

## API Selection

优先按这个顺序选入口：

1. `ScheduleJobByName(schedule, job, ...opts)`
   适合已有 `Job` 实现，直接复用 `job.Name()`。
2. `ScheduleJob(id, schedule, job, ...opts)`
   适合需要显式指定任务 ID。
3. `Schedule(id, schedule, handler, ...opts)`
   适合简单闭包任务。
4. helper API
   适合 one-shot 或有限次数：
   - `ScheduleOnceAt`
   - `ScheduleJobOnceAt`
   - `ScheduleLimited`
   - `ScheduleLimitedFrom`
   - `ScheduleLimitedJob`
   - `ScheduleLimitedJobFrom`

规则：

- helper API 优先于手写 `JobOptions{StartAt: ..., MaxRuns: ...}`
- helper API 会拒绝冲突的保留选项；不要在传入的 `JobOptions` 中重复设置不一致的 `StartAt` / `MaxRuns`
- 所有调度入口都最多只接受一个 `JobOptions`

## Call Patterns

### 最小函数任务

```go
c := cron.New()

if err := c.Schedule("sync-task", "@every 30s", func(ctx context.Context) {
    // do work
}); err != nil {
    return err
}

if err := c.Start(); err != nil {
    return err
}
defer c.StopGracefully(5 * time.Second)
```

### 推荐的 `Job` 任务

```go
type SyncJob struct{}

func (j *SyncJob) Name() string { return "sync-job" }

func (j *SyncJob) Run(ctx context.Context) error {
    return nil
}

c := cron.New()
err := c.ScheduleJobByName("@every 1m", &SyncJob{}, cron.JobOptions{
    Timeout: 10 * time.Second,
    Async:   true,
})
```

### 一次执行

```go
runAt := time.Now().Add(10 * time.Minute)
err := c.ScheduleOnceAt("warmup-once", runAt, func(ctx context.Context) {
    // run exactly once
})
```

### 指定首次执行时间的有限次任务

```go
startAt := time.Now().Add(30 * time.Minute)
err := c.ScheduleLimitedFrom("campaign-task", "@every 10m", startAt, 3, func(ctx context.Context) {
    // run at most 3 planned times
})
```

## Boundary Reminders

- `Pause` / `Resume` 只影响未来计划，不会中断已经在跑的执行。
- `MaxConcurrent > 0` 时，超过并发上限的触发会被跳过，不排队。
- `Timeout` 是单次尝试的超时，不是整轮重试总超时。
- `MaxRetries = -1` 表示无限重试；`0` 表示不重试。
- `MisfirePolicy` 真实关注 `skip`、`once`、`catchup` 三种。
- `GetTask` / `GetAllTasks` 看任务状态；`GetStats` / `GetAllStats` 看监控统计。
- 有限次 `Async` 任务的最后一次计划执行会先跑完，再自动移除；不要把自动移除误解成强制取消。
- 如果用户需要 `Stop -> Start` 重启同一个 `Cron`，不要提前 `Close()`。

## Runtime Lifecycle

- `Start()`：开始调度
- `Stop()`：立即停调度
- `StopGracefully(timeout)`：等待在途执行到超时
- `Close()`：终局动作，会停调度并关闭附着的 history recorder；之后实例不可复用

默认收口建议：

- 短生命周期程序：`defer c.Close()`
- 需要 stop/start 循环：先 `Stop()` 或 `StopGracefully()`，最后一次退出时再 `Close()`

## Load References Only When Needed

- parser 语法、`@every`、5 段/6 段表达式、`StartAt` 与 schedule 组合细节：
  读 [references/parser.md](references/parser.md)
- history recorder 接入、查询、关闭时机、stop/start 生命周期：
  读 [references/history.md](references/history.md)
- registry 模式、`RegisteredJob` 约束、实例注册表与全局注册表取舍：
  读 [references/registry.md](references/registry.md)

不要一次性加载全部 reference。按当前问题只读相关那一份。

## Suggested Workflow

1. 先确定任务形态：
   - 简单闭包
   - `Job` 接口
   - registry 批量注册
2. 再确定生命周期：
   - 常驻周期任务
   - one-shot
   - 有限次
3. 最后再加 `JobOptions`：
   - 只加当前需求真的需要的开关
   - 避免一次把 timeout、retry、async、misfire、fail-fuse 全塞进去
4. 改完后优先验证四类边界：
   - 快乐路径
   - 首次触发时间
   - 次数耗尽与自动移除
   - stop/close/history 生命周期

## Good Verification Targets

如果工作区包含 upstream 测试，优先查这些行为：

- `api_boundary_test.go`
  - helper API 冲突参数拒绝
  - `StartAt + MaxRuns` 的自动移除
  - `RunNow` 与运行态查询
  - `Close()` 的终局语义
- `history_test.go`
  - history 查询、统计、清理
  - `Stop()` 后再次 `Start()` 仍继续记 history
