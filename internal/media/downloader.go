package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrHostNotAllowed = errors.New("media host not allowed")
	ErrTooLarge       = errors.New("media too large")
	ErrInvalidType    = errors.New("invalid media MIME type")
)

type Kind int

const (
	Image Kind = iota
	Document
)

type Download struct {
	Path, MIMEType string
	Size           int64
}

type Downloader struct {
	allowed  map[string]struct{}
	max      int64
	resolver *net.Resolver
	tempDir  string
}

func NewDownloader(hosts []string, max int64) *Downloader {
	a := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		a[strings.ToLower(strings.TrimSuffix(h, "."))] = struct{}{}
	}
	return &Downloader{allowed: a, max: max, resolver: net.DefaultResolver}
}

func (d *Downloader) Fetch(ctx context.Context, rawURL string, kind Kind) (_ Download, retErr error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return Download{}, fmt.Errorf("%w: invalid URL", ErrHostNotAllowed)
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, ResponseHeaderTimeout: 15 * time.Second, DialContext: d.dialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return d.validateHost(req.Context(), req.URL.Hostname())
	}}
	if err = d.validateHost(ctx, u.Hostname()); err != nil {
		return Download{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Download{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Download{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Download{}, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	if resp.ContentLength > d.max {
		return Download{}, ErrTooLarge
	}
	f, err := os.CreateTemp(d.tempDir, "whatsmeow-media-*")
	if err != nil {
		return Download{}, err
	}
	path := f.Name()
	defer func() {
		if retErr != nil {
			_ = os.Remove(path)
		}
	}()
	buf := make([]byte, 512)
	n, readErr := io.ReadFull(resp.Body, buf)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		_ = f.Close()
		return Download{}, readErr
	}
	mime := http.DetectContentType(buf[:n])
	if !validMIME(kind, mime) {
		_ = f.Close()
		return Download{}, fmt.Errorf("%w: %s", ErrInvalidType, mime)
	}
	written, err := f.Write(buf[:n])
	if err == nil {
		var m int64
		m, err = io.Copy(f, io.LimitReader(resp.Body, d.max-int64(written)+1))
		written += int(m)
	}
	closeErr := f.Close()
	if err != nil {
		return Download{}, err
	}
	if closeErr != nil {
		return Download{}, closeErr
	}
	if int64(written) > d.max {
		return Download{}, ErrTooLarge
	}
	return Download{Path: path, MIMEType: mime, Size: int64(written)}, nil
}

func (d *Downloader) validateHost(_ context.Context, host string) error {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, ok := d.allowed[host]; !ok {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
	}
	return nil
}

func (d *Downloader) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if err = d.validateHost(ctx, host); err != nil {
		return nil, err
	}
	ips, err := d.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("media host has no IP address")
	}
	var last error
	dialer := net.Dialer{Timeout: 10 * time.Second}
	for _, ip := range ips {
		var c net.Conn
		c, last = dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if last == nil {
			return c, nil
		}
	}
	return nil, last
}

func validMIME(kind Kind, mime string) bool {
	mime = strings.ToLower(strings.Split(mime, ";")[0])
	if kind == Image {
		return strings.HasPrefix(mime, "image/")
	}
	return strings.HasPrefix(mime, "application/") || strings.HasPrefix(mime, "text/")
}

func Remove(download Download) error {
	if download.Path == "" {
		return nil
	}
	return os.Remove(filepath.Clean(download.Path))
}
