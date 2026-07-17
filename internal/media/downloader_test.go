package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func serverHost(t *testing.T, s *httptest.Server) string {
	t.Helper()
	u, _ := url.Parse(s.URL)
	return u.Hostname()
}

func TestFetchAndCleanup(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\ncontent"))
	}))
	defer s.Close()
	d := NewDownloader([]string{serverHost(t, s)}, 1024)
	got, err := d.Fetch(context.Background(), s.URL, Image)
	if err != nil {
		t.Fatal(err)
	}
	if got.MIMEType != "image/png" {
		t.Fatalf("mime %q", got.MIMEType)
	}
	if err = Remove(got); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(got.Path); !os.IsNotExist(err) {
		t.Fatalf("file not removed: %v", err)
	}
}

func TestHostAndRedirectBlocked(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("secret")) }))
	defer target.Close()
	blockedTarget := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, blockedTarget, http.StatusFound) }))
	defer redirect.Close()
	d := NewDownloader([]string{serverHost(t, redirect)}, 1024)
	_, err := d.Fetch(context.Background(), redirect.URL, Document)
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("expected host error, got %v", err)
	}
	d = NewDownloader(nil, 1024)
	_, err = d.Fetch(context.Background(), target.URL, Document)
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("expected host error, got %v", err)
	}
}

func TestLimitAndMIME(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(strings.Repeat("x", 20)))
	}))
	defer s.Close()
	d := NewDownloader([]string{serverHost(t, s)}, 10)
	_, err := d.Fetch(context.Background(), s.URL, Document)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected size error, got %v", err)
	}
	d = NewDownloader([]string{serverHost(t, s)}, 100)
	_, err = d.Fetch(context.Background(), s.URL, Image)
	if !errors.Is(err, ErrInvalidType) {
		t.Fatalf("expected MIME error, got %v", err)
	}
}
