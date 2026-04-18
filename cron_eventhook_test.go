package cron

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEventHookCalled(t *testing.T) {
	var mu sync.Mutex
	events := make([]Event, 0, 2)
	hook := func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}

	c := New(WithEventHook(hook))

	job := func(ctx context.Context) { time.Sleep(10 * time.Millisecond) }

	if err := c.Schedule("hook-test", EverySecond, job); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer c.StopGracefully(1 * time.Second)

	// 立即触发一次
	if err := c.RunNow("hook-test"); err != nil {
		t.Fatalf("run now failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	mu.Lock()
	count := len(events)
	mu.Unlock()

	if count == 0 {
		t.Fatalf("expected event hook to be called, got %d", count)
	}
}

func TestNewEventLoggerHook_NilLogger(t *testing.T) {
	hook := NewEventLoggerHook(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("hook should not panic with nil logger: %v", r)
		}
	}()

	hook(Event{TaskID: "nil-logger", End: time.Now(), Success: true})
}
