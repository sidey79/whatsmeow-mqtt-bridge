package whatsapp

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
)

type orderedHandler struct {
	mu   sync.Mutex
	ids  []string
	done chan struct{}
}

func (h *orderedHandler) OnMessage(evt model.MessageEvent) {
	h.mu.Lock()
	h.ids = append(h.ids, evt.MessageID)
	if len(h.ids) == 3 {
		close(h.done)
	}
	h.mu.Unlock()
}
func (*orderedHandler) OnConnected(string)    {}
func (*orderedHandler) OnDisconnected(string) {}
func (*orderedHandler) OnLoggedOut(string)    {}

func TestMessageWorkerPreservesOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &orderedHandler{done: make(chan struct{})}
	c := &Client{handler: h, eventCtx: ctx, messages: make(chan model.MessageEvent, 3)}
	go c.messageWorker()
	for _, id := range []string{"one", "two", "three"} {
		c.messages <- model.MessageEvent{MessageID: id}
	}
	<-h.done
	h.mu.Lock()
	defer h.mu.Unlock()
	if !reflect.DeepEqual(h.ids, []string{"one", "two", "three"}) {
		t.Fatalf("order: %v", h.ids)
	}
}
