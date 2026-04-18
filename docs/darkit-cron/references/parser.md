# Parser Reference

当用户问题落在 schedule 语法、`@every`、`StartAt`、首次触发时间、有限次计划是否过期时，再读本文件。

## Accepted Schedule Forms

`darkit/cron` 的入口调度解析分两路：

- 5 段表达式：走标准 cron 解析
  - 形态：`minute hour dom month dow`
- 其他表达式：走带 seconds 和 descriptor 的 parser
  - 可处理 6 段表达式
  - 可处理 descriptor，如 `@monthly`、`@weekly`
  - 可处理 `@every 1h30m`

如果表达式不合法，调度时会直接报错；registry 注册时也会提前校验 `Schedule() string`。

## `@every` Semantics

`@every` 不是按自然日历点位对齐，而是 constant delay。

关键语义：

- 未设置 `StartAt`：
  - 首次触发点接近“现在”
- 设置未来 `StartAt`：
  - 第一次计划执行在 `StartAt`
- 设置过去 `StartAt`：
  - scheduler 会根据 delay 计算已经跨过多少个 step
  - 有限次任务会同步扣减剩余计划次数
  - 如果所有计划次数都已经过期，任务会被判定为 expired

这意味着：

- `StartAt` 对 `@every` 任务不是装饰项，而是真正的时间锚点
- 想要“从某个时刻开始，每隔 N 分钟跑 M 次”，优先用 helper API，而不是手算 cron 表达式

## Non-`@every` Semantics

对标准 cron schedule：

- `StartAt` 表示“从这个时间开始找第一个合法触发点”
- 如果 `StartAt` 本身不落在表达式上，会取“`StartAt` 之后第一个匹配点”
- 对有限次任务，scheduler 会跳过已经过去的计划点，并扣减相应剩余次数
- 如果扣减后没有剩余计划次数，任务会被视为 expired，不会加入调度器

## `StartAt + MaxRuns`

真实语义要点：

- `MaxRuns` 只统计计划执行次数
- `RunNow(id)` 是额外执行，不消耗 `MaxRuns`
- 有限次任务计划次数耗尽后会自动移除
- 有限次 `Async` 任务会等最后一次在途执行结束，再完成自动移除

## Debug Checklist

看到“为什么没触发 / 为什么提前结束 / 为什么被拒绝”时，优先检查：

1. schedule 解析走的是 5 段还是 descriptor/seconds 路线
2. 是否给了过去的 `StartAt`
3. `MaxRuns` 是否已经在计划推演时被扣完
4. 是否把 `RunNow` 错当成计划次数的一部分

如果当前工作区带有 upstream 源码，优先核对：

- `scheduler.go` 中的 `parseSchedule`
- `scheduler.go` 中围绕 `planInitialState`、`recomputeNextRun` 的逻辑
- `api_boundary_test.go` 中有限次任务相关用例
