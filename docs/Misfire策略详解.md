# Misfire 策略详解

Misfire（错过触发）是指任务的预定执行时间已过，但由于各种原因（如系统繁忙、任务执行时间过长等）导致任务未能按时执行的情况。本文详细介绍 Cron 库提供的三种 Misfire 处理策略。

## 什么是 Misfire

### Misfire 发生场景

当以下情况出现时，可能会发生 Misfire：

1. **任务执行时间过长**：上一次执行还未完成，下一次执行时间已到
2. **系统资源不足**：CPU、内存等资源紧张，任务无法及时启动
3. **并发限制**：达到 `MaxConcurrent` 限制，新的执行被阻塞
4. **调度器暂停**：调度器被暂停或停止后重新启动
5. **任务暂停后恢复**：任务暂停期间错过了多次执行机会

### 示例场景

假设有一个任务配置为每分钟执行一次（`0 * * * * *`），但每次执行需要2分钟：

```
10:00:00 - 任务开始执行
10:02:00 - 任务完成
         - 此时已经错过了 10:01:00 的执行时间（Misfire）
```

## 三种 Misfire 策略

Cron 库提供三种策略来处理 Misfire 情况：

### 1. Skip 策略（跳过）

**策略标识：** `MisfireSkip`（默认策略）

**行为：** 跳过所有错过的执行，直接调度到下一个有效的执行时间。

**适用场景：**
- 实时性要求高的任务（如监控、告警）
- 历史数据无需补偿的场景
- 避免系统过载

**示例：**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/darkit/cron"
)

func main() {
    c := cron.New()

    // 配置 Skip 策略：错过的执行直接跳过
    c.Schedule("monitor", "*/10 * * * * *", func(ctx context.Context) {
        fmt.Printf("[%s] 监控任务执行\n", time.Now().Format("15:04:05"))
        time.Sleep(25 * time.Second) // 模拟耗时任务
    }, cron.JobOptions{
        MisfirePolicy: cron.MisfireSkip,
    })

    c.Start()
    defer c.Stop()

    time.Sleep(2 * time.Minute)
}
```

**执行时间线：**

```
00:00 - 任务开始（执行需要25秒）
00:10 - 应该执行但被跳过（任务还在执行中）
00:20 - 应该执行但被跳过（任务还在执行中）
00:25 - 上次任务完成
00:30 - 下一次正常执行
```

### 2. Once 策略（补跑一次）

**策略标识：** `MisfireRunOnce`

**行为：** 如果错过了执行，立即补跑一次，然后恢复正常调度。

**适用场景：**
- 需要保证至少执行一次的任务
- 数据同步任务
- 重要但不需要全量补偿的场景

**示例：**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/darkit/cron"
)

func main() {
    c := cron.New()

    // 配置 Once 策略：错过后补跑一次
    c.Schedule("sync", "*/10 * * * * *", func(ctx context.Context) {
        fmt.Printf("[%s] 数据同步任务执行\n", time.Now().Format("15:04:05"))
        time.Sleep(25 * time.Second) // 模拟耗时任务
    }, cron.JobOptions{
        MisfirePolicy: cron.MisfireRunOnce,
        Async:         true, // 异步执行避免阻塞
    })

    c.Start()
    defer c.Stop()

    time.Sleep(2 * time.Minute)
}
```

**执行时间线：**

```
00:00 - 任务开始（异步执行，需要25秒）
00:10 - 应该执行但错过（任务还在执行中）
00:20 - 应该执行但错过（任务还在执行中）
00:25 - 上次任务完成
00:25 - 立即补跑一次（因为错过了多次执行）
00:30 - 恢复正常调度，按时执行
```

### 3. CatchUp 策略（追赶）

**策略标识：** `MisfireCatchUp`

**行为：** 尽可能补跑所有错过的执行（最多5次），然后恢复正常调度。

**适用场景：**
- 数据处理任务，需要保证完整性
- 批处理任务
- 增量同步任务

**示例：**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/darkit/cron"
)

func main() {
    c := cron.New()

    // 配置 CatchUp 策略：尽可能补跑错过的执行
    c.Schedule("batch", "*/10 * * * * *", func(ctx context.Context) {
        fmt.Printf("[%s] 批处理任务执行\n", time.Now().Format("15:04:05"))
        time.Sleep(3 * time.Second) // 快速处理
    }, cron.JobOptions{
        MisfirePolicy: cron.MisfireCatchUp,
        Async:         true, // 异步执行提高效率
    })

    c.Start()

    // 模拟系统暂停
    time.Sleep(15 * time.Second)
    fmt.Println("暂停任务...")
    c.Pause("batch")

    // 暂停30秒（错过3次执行）
    time.Sleep(30 * time.Second)

    fmt.Println("恢复任务...")
    c.Resume("batch")

    // 观察补跑
    time.Sleep(1 * time.Minute)
    c.Stop()
}
```

**执行时间线：**

```
00:00 - 任务执行
00:10 - 任务执行
00:15 - 任务被暂停
00:20 - 错过
00:30 - 错过
00:40 - 错过
00:45 - 任务恢复
00:45 - 立即补跑第1次（对应00:20）
00:45 - 立即补跑第2次（对应00:30）
00:45 - 立即补跑第3次（对应00:40）
00:50 - 恢复正常调度
```

## 策略对比

### 特性对比表

| 特性 | Skip | Once | CatchUp |
|------|------|------|---------|
| 错过后执行次数 | 0 次 | 1 次 | 最多 5 次 |
| 系统负载 | 最低 | 中等 | 较高 |
| 数据完整性 | 可能丢失 | 基本保证 | 完全保证 |
| 实时性 | 最好 | 较好 | 较差 |
| 适用任务类型 | 监控、告警 | 同步、更新 | 批处理、数据处理 |

### 性能影响

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/darkit/cron"
)

func benchmark() {
    policies := []struct {
        name   string
        policy cron.MisfirePolicy
    }{
        {"Skip", cron.MisfireSkip},
        {"Once", cron.MisfireRunOnce},
        {"CatchUp", cron.MisfireCatchUp},
    }

    for _, p := range policies {
        fmt.Printf("\n测试策略: %s\n", p.name)

        c := cron.New()
        execCount := 0

        c.Schedule("test", "*/5 * * * * *", func(ctx context.Context) {
            execCount++
            time.Sleep(20 * time.Second) // 模拟耗时任务
        }, cron.JobOptions{
            MisfirePolicy: p.policy,
            Async:         true,
        })

        c.Start()
        time.Sleep(1 * time.Minute)
        c.Stop()

        fmt.Printf("执行次数: %d\n", execCount)
    }
}

func main() {
    benchmark()
}
```

## 配置建议

### 1. 监控类任务（推荐 Skip）

```go
// 系统监控：只关注当前状态，历史数据无意义
c.Schedule("health-check", "@every 30s", healthCheckFunc, cron.JobOptions{
    MisfirePolicy: cron.MisfireSkip,
    Timeout:       5 * time.Second,
})
```

### 2. 数据同步任务（推荐 Once）

```go
// 数据同步：保证至少同步一次，避免数据长时间不一致
c.Schedule("data-sync", "@every 5m", syncDataFunc, cron.JobOptions{
    MisfirePolicy: cron.MisfireRunOnce,
    Async:         true,
    Timeout:       2 * time.Minute,
})
```

### 3. 批处理任务（推荐 CatchUp）

```go
// 批处理：需要处理所有时间段的数据
c.Schedule("batch-process", "@every 1h", batchProcessFunc, cron.JobOptions{
    MisfirePolicy: cron.MisfireCatchUp,
    Async:         true,
    Timeout:       30 * time.Minute,
})
```

### 4. 定时报表（推荐 Once）

```go
// 报表生成：只需要最新的报表，但必须生成
c.Schedule("daily-report", "0 0 8 * * *", generateReportFunc, cron.JobOptions{
    MisfirePolicy: cron.MisfireRunOnce,
    Timeout:       10 * time.Minute,
})
```

## 完整示例

### 示例1：多策略对比演示

```go
package main

import (
    "context"
    "fmt"
    "sync/atomic"
    "time"

    "github.com/darkit/cron"
)

func main() {
    c := cron.New()

    // 计数器
    var skipCount, onceCount, catchUpCount int32

    // Skip 策略任务
    c.Schedule("skip-task", "*/10 * * * * *", func(ctx context.Context) {
        atomic.AddInt32(&skipCount, 1)
        fmt.Printf("[Skip] 执行第 %d 次\n", atomic.LoadInt32(&skipCount))
        time.Sleep(25 * time.Second)
    }, cron.JobOptions{
        MisfirePolicy: cron.MisfireSkip,
    })

    // Once 策略任务
    c.Schedule("once-task", "*/10 * * * * *", func(ctx context.Context) {
        atomic.AddInt32(&onceCount, 1)
        fmt.Printf("[Once] 执行第 %d 次\n", atomic.LoadInt32(&onceCount))
        time.Sleep(25 * time.Second)
    }, cron.JobOptions{
        MisfirePolicy: cron.MisfireRunOnce,
        Async:         true,
    })

    // CatchUp 策略任务
    c.Schedule("catchup-task", "*/10 * * * * *", func(ctx context.Context) {
        atomic.AddInt32(&catchUpCount, 1)
        fmt.Printf("[CatchUp] 执行第 %d 次\n", atomic.LoadInt32(&catchUpCount))
        time.Sleep(3 * time.Second)
    }, cron.JobOptions{
        MisfirePolicy: cron.MisfireCatchUp,
        Async:         true,
    })

    c.Start()

    // 运行2分钟
    time.Sleep(2 * time.Minute)

    c.Stop()

    // 输出统计
    fmt.Println("\n执行统计：")
    fmt.Printf("Skip 策略: %d 次\n", atomic.LoadInt32(&skipCount))
    fmt.Printf("Once 策略: %d 次\n", atomic.LoadInt32(&onceCount))
    fmt.Printf("CatchUp 策略: %d 次\n", atomic.LoadInt32(&catchUpCount))
}
```

### 示例2：实际业务场景

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/darkit/cron"
)

type DataSyncJob struct {
    lastSyncTime time.Time
}

func (j *DataSyncJob) Name() string {
    return "data-sync"
}

func (j *DataSyncJob) Run(ctx context.Context) error {
    now := time.Now()
    fmt.Printf("同步数据: %s -> %s\n",
        j.lastSyncTime.Format("15:04:05"),
        now.Format("15:04:05"))

    // 模拟数据同步
    time.Sleep(5 * time.Second)

    j.lastSyncTime = now
    return nil
}

func main() {
    c := cron.New()

    syncJob := &DataSyncJob{
        lastSyncTime: time.Now(),
    }

    // 使用 Once 策略确保同步不会丢失
    c.ScheduleJobByName("@every 10s", syncJob, cron.JobOptions{
        MisfirePolicy: cron.MisfireRunOnce,
        Async:         true,
        MaxRetries:    3,
        RetryInterval: 2 * time.Second,
    })

    c.Start()
    defer c.StopGracefully(10 * time.Second)

    // 模拟系统繁忙导致 Misfire
    fmt.Println("系统运行正常...")
    time.Sleep(30 * time.Second)

    fmt.Println("系统暂停（模拟维护）...")
    c.Pause("data-sync")
    time.Sleep(30 * time.Second)

    fmt.Println("系统恢复...")
    c.Resume("data-sync")

    // 观察补跑
    time.Sleep(1 * time.Minute)
}
```

## 与其他功能的配合

### 与异步执行配合

```go
// 推荐：CatchUp 策略配合异步执行
c.Schedule("task", "*/5 * * * * *", handler, cron.JobOptions{
    MisfirePolicy: cron.MisfireCatchUp,
    Async:         true, // 避免阻塞调度器
    MaxConcurrent: 3,    // 限制并发补跑数量
})
```

### 与重试机制配合

```go
// Misfire 补跑失败后自动重试
c.Schedule("task", "@every 10m", handler, cron.JobOptions{
    MisfirePolicy: cron.MisfireRunOnce,
    MaxRetries:    3,
    RetryInterval: 1 * time.Minute,
})
```

### 与失败熔断配合

```go
// 防止补跑导致系统过载
c.Schedule("task", "@every 5m", handler, cron.JobOptions{
    MisfirePolicy: cron.MisfireCatchUp,
    FailThreshold: 5,              // 连续失败5次触发熔断
    FailWindow:    10 * time.Minute, // 10分钟内
    PauseDuration: 30 * time.Minute, // 暂停30分钟
})
```

## 监控 Misfire

使用事件钩子监控 Misfire 情况：

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/darkit/cron"
)

func main() {
    // 自定义事件钩子监控 Misfire
    hook := func(ev cron.Event) {
        if ev.End.IsZero() {
            return // 忽略开始事件
        }

        // 检查任务是否补跑（通过执行间隔判断）
        // 实际应用中可以记录上次执行时间进行对比
        if !ev.Success {
            log.Printf("[Misfire] Task %s failed after misfire, retries=%d",
                ev.TaskID, ev.Retries)
        }
    }

    c := cron.New(cron.WithEventHook(hook))

    // ... 添加任务 ...

    c.Start()
    defer c.Stop()

    time.Sleep(5 * time.Minute)
}
```

## 常见问题

### Q: 如何选择合适的 Misfire 策略？

A: 根据任务特点选择：
- **实时性优先**：使用 Skip
- **至少执行一次**：使用 Once
- **数据完整性优先**：使用 CatchUp

### Q: CatchUp 策略为什么限制最多补跑5次？

A: 防止系统长时间暂停后，大量补跑任务造成系统过载。如果需要更多补跑，可以分批处理或使用历史记录功能。

### Q: Misfire 策略会影响手动触发的任务吗？

A: 不会。`RunNow()` 手动触发的任务不受 Misfire 策略影响。

### Q: 如何避免 Misfire 的发生？

A:
- 合理设置任务超时时间
- 使用异步执行避免阻塞
- 设置合适的 `MaxConcurrent` 限制
- 优化任务执行效率
- 监控系统资源使用情况

### Q: Misfire 策略为空时使用什么默认值？

A: 默认使用 `MisfireSkip` 策略。

## 最佳实践

1. **明确指定策略**：不要依赖默认值，明确指定 `MisfirePolicy`
2. **配合异步执行**：CatchUp 和 Once 策略建议配合 `Async: true`
3. **设置并发限制**：使用 `MaxConcurrent` 避免补跑导致系统过载
4. **监控补跑情况**：使用事件钩子或历史记录监控 Misfire
5. **测试验证**：在测试环境模拟 Misfire 场景，验证策略效果

## 相关文档

- [运行时控制](./运行时控制.md) - 动态管理任务执行
- [失败熔断](./失败熔断.md) - 任务失败保护机制
- [事件钩子](./事件钩子.md) - 监控任务执行
