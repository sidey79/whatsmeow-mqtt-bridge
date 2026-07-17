package bridge

import (
	"context"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/media"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
	"log/slog"
	"testing"
	"time"
)

type fakePub struct {
	delivery []model.DeliveryEvent
	errs     []model.ErrorEvent
	status   []model.StatusEvent
}

func (f *fakePub) PublishStatus(_ context.Context, v model.StatusEvent) error {
	f.status = append(f.status, v)
	return nil
}
func (f *fakePub) PublishMessage(context.Context, model.MessageEvent) error { return nil }
func (f *fakePub) PublishDelivery(_ context.Context, v model.DeliveryEvent) error {
	f.delivery = append(f.delivery, v)
	return nil
}
func (f *fakePub) PublishError(_ context.Context, v model.ErrorEvent) error {
	f.errs = append(f.errs, v)
	return nil
}
func (f *fakePub) Connected() bool { return true }

type fakeWA struct{ ready bool }

func (f *fakeWA) Ready() bool   { return f.ready }
func (f *fakeWA) Phone() string { return "491701234567" }
func (f *fakeWA) SendText(_ context.Context, _ string, _ string) (string, time.Time, error) {
	return "3EB0", time.Unix(1, 0), nil
}
func (f *fakeWA) SendMedia(context.Context, string, string, string, string, string, bool) (string, time.Time, error) {
	return "", time.Time{}, nil
}
func (f *fakeWA) Reconnect(context.Context) error { return nil }

type fakeDL struct{}

func (fakeDL) Fetch(context.Context, string, media.Kind) (media.Download, error) {
	return media.Download{}, nil
}
func TestTextContract(t *testing.T) {
	p := &fakePub{}
	b := New(p, &fakeWA{ready: true}, fakeDL{}, slog.Default())
	b.HandleCommand(context.Background(), "send/text", []byte(`{"requestId":"req-123","to":"491701234567","payload":{"text":"Hallo"}}`))
	if len(p.delivery) != 1 || p.delivery[0].RequestID != "req-123" || p.delivery[0].MessageID != "3EB0" || len(p.errs) != 0 {
		t.Fatalf("delivery=%+v errors=%+v", p.delivery, p.errs)
	}
}
func TestInvalidAndNotReadyExactlyOneError(t *testing.T) {
	for name, data := range map[string]string{"invalid": `{"requestId":"x","to":"+49170","payload":{"text":"hi"}}`, "unknown": `{"requestId":"x","to":"491701234567","extra":1,"payload":{"text":"hi"}}`} {
		t.Run(name, func(t *testing.T) {
			p := &fakePub{}
			b := New(p, &fakeWA{ready: true}, fakeDL{}, slog.Default())
			b.HandleCommand(context.Background(), "send/text", []byte(data))
			if len(p.errs) != 1 || len(p.delivery) != 0 {
				t.Fatalf("results: %+v %+v", p.errs, p.delivery)
			}
		})
	}
	p := &fakePub{}
	b := New(p, &fakeWA{}, fakeDL{}, slog.Default())
	b.HandleCommand(context.Background(), "send/text", []byte(`{"requestId":"x","to":"491701234567","payload":{"text":"hi"}}`))
	if len(p.errs) != 1 || p.errs[0].ErrorCode != model.ErrorNotReady {
		t.Fatalf("%+v", p.errs)
	}
}
