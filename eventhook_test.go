package cron

import (
	"testing"
	"time"
)

func TestNewEventChannelHookNonBlockingWhenChannelFull(t *testing.T) {
	ch := make(chan Event, 1)
	hook := NewEventChannelHook(ch)
	ch <- Event{TaskID: "first"}
	start := time.Now()
	hook(Event{TaskID: "second"})
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("expected hook to be non-blocking when channel is full")
	}
}
