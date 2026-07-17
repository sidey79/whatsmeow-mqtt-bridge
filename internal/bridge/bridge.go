package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sven/whatsmeow-mqtt-bridge/internal/media"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
)

type Publisher interface {
	PublishStatus(context.Context, model.StatusEvent) error
	PublishMessage(context.Context, model.MessageEvent) error
	PublishDelivery(context.Context, model.DeliveryEvent) error
	PublishError(context.Context, model.ErrorEvent) error
	Connected() bool
}
type WhatsApp interface {
	Ready() bool
	Phone() string
	SendText(context.Context, string, string) (string, time.Time, error)
	SendMedia(context.Context, string, string, string, string, string, bool) (string, time.Time, error)
	Reconnect(context.Context) error
}
type Downloader interface {
	Fetch(context.Context, string, media.Kind) (media.Download, error)
}

type mediaDownloadError struct{ err error }

func (e *mediaDownloadError) Error() string { return e.err.Error() }
func (e *mediaDownloadError) Unwrap() error { return e.err }

type Bridge struct {
	pub        Publisher
	wa         WhatsApp
	downloader Downloader
	log        *slog.Logger
	sem        chan struct{}
	status     atomic.Value
	statusMu   sync.Mutex
}

func New(pub Publisher, wa WhatsApp, d Downloader, log *slog.Logger) *Bridge {
	b := &Bridge{pub: pub, wa: wa, downloader: d, log: log, sem: make(chan struct{}, 4)}
	b.status.Store(model.StatusEvent{State: model.StateStarting})
	return b
}

func (b *Bridge) Status() model.StatusEvent { return b.status.Load().(model.StatusEvent) }
func (b *Bridge) setStatus(ctx context.Context, s model.StatusEvent) {
	b.statusMu.Lock()
	b.status.Store(s)
	err := b.pub.PublishStatus(ctx, s)
	b.statusMu.Unlock()
	if err != nil {
		b.log.Warn("status publish failed", "error", err)
	}
}
func (b *Bridge) SetConnecting(ctx context.Context) {
	b.setStatus(ctx, model.StatusEvent{State: model.StateConnecting, Connected: false, Phone: b.wa.Phone()})
}
func (b *Bridge) PublishCurrentStatus(ctx context.Context) {
	s := b.Status()
	_ = b.pub.PublishStatus(ctx, s)
}
func (b *Bridge) Ready() bool { return b.pub.Connected() && b.wa.Ready() }
func (b *Bridge) OnConnected(phone string) {
	b.setStatus(context.Background(), model.StatusEvent{State: model.StateReady, Connected: true, Phone: phone})
}
func (b *Bridge) OnDisconnected(message string) {
	b.setStatus(context.Background(), model.StatusEvent{State: model.StateDisconnected, Connected: false, Phone: b.wa.Phone(), Message: message})
}
func (b *Bridge) OnLoggedOut(message string) {
	b.setStatus(context.Background(), model.StatusEvent{State: model.StateError, Connected: false, Phone: "", Message: message})
}
func (b *Bridge) OnMessage(evt model.MessageEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := b.pub.PublishMessage(ctx, evt); err != nil {
		b.log.Error("incoming message publish failed", "error", err)
	}
}

func (b *Bridge) HandleCommand(ctx context.Context, kind string, data []byte) {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
	case <-ctx.Done():
		return
	}
	b.handle(ctx, kind, data)
}
func (b *Bridge) handle(ctx context.Context, kind string, data []byte) {
	var env model.CommandEnvelope
	if len(data) == 0 || decodeStrict(data, &env) != nil || strings.TrimSpace(env.RequestID) == "" {
		b.fail(ctx, env.RequestID, model.ErrorInvalidPayload, "Invalid command envelope")
		return
	}
	if kind == "status" {
		b.PublishCurrentStatus(ctx)
		return
	}
	if kind == "reconnect" {
		b.setStatus(ctx, model.StatusEvent{State: model.StateConnecting, Phone: b.wa.Phone()})
		if err := b.wa.Reconnect(ctx); err != nil {
			b.fail(ctx, env.RequestID, model.ErrorWhatsAppDisconnected, err.Error())
		}
		return
	}
	if !b.wa.Ready() {
		b.fail(ctx, env.RequestID, model.ErrorNotReady, "WhatsApp client is not ready")
		return
	}
	if _, err := recipient(env.To); err != nil {
		b.fail(ctx, env.RequestID, model.ErrorInvalidPayload, err.Error())
		return
	}
	var id string
	var ts time.Time
	var err error
	switch kind {
	case "send/text":
		var p model.TextPayload
		if decodeStrict(env.Payload, &p) != nil || strings.TrimSpace(p.Text) == "" {
			b.fail(ctx, env.RequestID, model.ErrorInvalidPayload, "Text payload is invalid")
			return
		}
		id, ts, err = b.wa.SendText(ctx, env.To, p.Text)
	case "send/image":
		var p model.ImagePayload
		if decodeStrict(env.Payload, &p) != nil || p.URL == "" {
			b.fail(ctx, env.RequestID, model.ErrorInvalidPayload, "Image payload is invalid")
			return
		}
		id, ts, err = b.sendMedia(ctx, env.To, p.URL, p.Caption, "", true)
	case "send/document":
		var p model.DocumentPayload
		if decodeStrict(env.Payload, &p) != nil || p.URL == "" {
			b.fail(ctx, env.RequestID, model.ErrorInvalidPayload, "Document payload is invalid")
			return
		}
		id, ts, err = b.sendMedia(ctx, env.To, p.URL, p.Title, p.FileName, false)
	default:
		b.fail(ctx, env.RequestID, model.ErrorInvalidPayload, "Unknown command")
		return
	}
	if err != nil {
		code := model.ErrorSendFailed
		if errors.Is(err, media.ErrHostNotAllowed) {
			code = model.ErrorMediaHostNotAllowed
		} else if errors.Is(err, media.ErrTooLarge) {
			code = model.ErrorMediaTooLarge
		} else {
			var downloadErr *mediaDownloadError
			if errors.As(err, &downloadErr) {
				code = model.ErrorMediaDownloadFailed
			}
		}
		b.fail(ctx, env.RequestID, code, err.Error())
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	_ = b.pub.PublishDelivery(ctx, model.DeliveryEvent{RequestID: env.RequestID, Status: "ok", MessageID: id, Timestamp: model.UTCMillis(ts)})
}

func (b *Bridge) sendMedia(ctx context.Context, to, url, caption, fileName string, image bool) (string, time.Time, error) {
	kind := media.Document
	if image {
		kind = media.Image
	}
	download, err := b.downloader.Fetch(ctx, url, kind)
	if err != nil {
		return "", time.Time{}, &mediaDownloadError{err: err}
	}
	defer func() {
		if err := media.Remove(download); err != nil {
			b.log.Warn("temporary media cleanup failed", "error", err)
		}
	}()
	return b.wa.SendMedia(ctx, to, download.Path, download.MIMEType, caption, fileName, image)
}
func (b *Bridge) fail(ctx context.Context, requestID, code, message string) {
	_ = b.pub.PublishError(ctx, model.ErrorEvent{RequestID: requestID, Status: "error", ErrorCode: code, Message: message, Timestamp: model.UTCMillis(time.Now())})
}
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}
func recipient(s string) (string, error) {
	if len(s) < 6 || len(s) > 15 {
		return "", fmt.Errorf("recipient must contain 6 to 15 digits")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", errors.New("recipient must contain digits without leading +")
		}
	}
	return s, nil
}
