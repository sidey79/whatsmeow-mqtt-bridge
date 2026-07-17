package mqttbridge

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCommandWorkersBoundConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	c := &Client{ctx: ctx, commands: make(chan command, commandQueueSize), handler: func(context.Context, string, []byte) {
		current := active.Add(1)
		for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
		}
		<-release
		active.Add(-1)
	}}
	for range commandWorkers {
		go c.commandWorker()
	}
	var senders sync.WaitGroup
	for range commandWorkers * 3 {
		senders.Add(1)
		go func() { defer senders.Done(); c.enqueue(command{kind: "send/text"}) }()
	}
	time.Sleep(50 * time.Millisecond)
	if got := maximum.Load(); got > commandWorkers {
		t.Fatalf("maximum concurrency %d exceeds %d", got, commandWorkers)
	}
	close(release)
	senders.Wait()
}
