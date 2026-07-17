package bridge

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/sven/whatsmeow-mqtt-bridge/internal/media"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
)

type failingMediaWA struct{ fakeWA }

func (f *failingMediaWA) SendMedia(context.Context, string, string, string, string, string, bool) (string, time.Time, error) {
	return "", time.Time{}, errors.New("upload failed")
}

type failingDownloader struct{}

func (failingDownloader) Fetch(context.Context, string, media.Kind) (media.Download, error) {
	return media.Download{}, errors.New("download failed")
}

func TestMediaSendAndDownloadErrorsAreDistinct(t *testing.T) {
	command := []byte(`{"requestId":"media-1","to":"491701234567","payload":{"url":"https://example.com/image.png"}}`)

	pub := &fakePub{}
	b := New(pub, &failingMediaWA{fakeWA{ready: true}}, fakeDL{}, slog.Default())
	b.HandleCommand(context.Background(), "send/image", command)
	if len(pub.errs) != 1 || pub.errs[0].ErrorCode != model.ErrorSendFailed {
		t.Fatalf("upload/send error classified as %+v", pub.errs)
	}

	pub = &fakePub{}
	b = New(pub, &fakeWA{ready: true}, failingDownloader{}, slog.Default())
	b.HandleCommand(context.Background(), "send/image", command)
	if len(pub.errs) != 1 || pub.errs[0].ErrorCode != model.ErrorMediaDownloadFailed {
		t.Fatalf("download error classified as %+v", pub.errs)
	}
}

func TestSetConnectingUpdatesCurrentStatus(t *testing.T) {
	pub := &fakePub{}
	b := New(pub, &fakeWA{}, fakeDL{}, slog.Default())
	b.SetConnecting(context.Background())
	if got := b.Status(); got.State != model.StateConnecting || got.Phone != "491701234567" {
		t.Fatalf("unexpected status: %+v", got)
	}
}
