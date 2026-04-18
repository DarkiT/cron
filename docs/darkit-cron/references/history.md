# History Reference

当用户问题落在 history recorder 接入、查询、清理、关闭时机、`Stop`/`Start` 生命周期时，再读本文件。

## Enable History

history 不是默认开启的，需要显式注入 recorder：

```go
storage, err := history.NewFileStorage("./history")
if err != nil {
    return err
}

recorder := history.NewHistoryRecorder(storage)

c := cron.New(
    cron.WithHistoryRecorder(recorder),
)
```

如果没有注入 recorder：

- `QueryHistory`
- `CountHistory`
- `CleanupHistory`

这些 API 会直接返回 error。

## Lifecycle Rules

这是最容易踩坑的部分。

- `WithHistoryRecorder(recorder)` 只把 recorder 挂到 `Cron`
- `Stop()` 不会关闭 recorder
- `StopGracefully()` 也不会关闭 recorder
- `Close()` 才会在终局阶段关闭 recorder

因此：

- 如果需要 `Stop -> Start` 复用同一个 `Cron`，不要关闭 recorder
- 如果程序真的结束了，再调用 `Close()` 做总收口

推荐模式：

```go
recorder := history.NewHistoryRecorder(storage)
defer recorder.Close() // 仅在你自己完全持有 recorder 生命周期时使用

c := cron.New(cron.WithHistoryRecorder(recorder))
defer c.Close()
```

如果 `Cron` 已经负责最终 `Close()`，就不要再对同一个 recorder 重复做复杂的外层生命周期编排。

## Recorder Behavior

`HistoryRecorder` 本身有几个实现细节值得记住：

- 写入是异步队列模式
- 队列默认缓冲 `100`
- 队列满时会退化成同步保存，避免静默丢数据
- `Close()` 后新的 `Record(...)` 会直接 no-op
- `Close()` 会等待入队和后台写入收尾，再关闭底层 storage

这意味着：

- history 一般不会阻塞主调度线程
- 但极端高压下仍可能回退到同步写入
- 测试里看到“等一小段时间再查 history”通常是为了等异步写入落盘

## Query Surface

对外主要是三类查询：

- `QueryHistory(filter)`
- `CountHistory(filter)`
- `CleanupHistory(before)`

排查时先确认：

1. recorder 是否启用
2. 任务是否真的执行过
3. 是否给了足够时间让异步写入完成

## Verification Hints

如果当前工作区带有 upstream 测试，优先核对：

- `history_test.go` 中的查询、统计、清理用例
- `history_test.go` 中 `Stop()` 后再次 `Start()` 仍继续记 history 的用例
- `api_boundary_test.go` 中 `Close()` 关闭 recorder 的用例
