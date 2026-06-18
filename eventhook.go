package cron

// NewEventLoggerHook 返回简单的事件日志钩子，仅在任务结束时记录结果
func NewEventLoggerHook(l Logger) EventHook {
	if l == nil {
		l = &NoOpLogger{}
	}

	return func(ev Event) {
		if ev.End.IsZero() {
			return // 开始事件不打日志，避免噪声
		}
		if ev.Success {
			l.Infof("task %s done, duration=%s, retries=%d", ev.TaskID, ev.Duration, ev.Retries)
			return
		}
		l.Warnf("task %s failed, duration=%s, retries=%d, err=%s", ev.TaskID, ev.Duration, ev.Retries, ev.Error)
	}
}

// NewEventChannelHook 将事件推送到外部 channel，便于指标/审计
func NewEventChannelHook(ch chan<- Event) EventHook {
	return func(ev Event) {
		select {
		case ch <- ev:
		default:
		}
	}
}
