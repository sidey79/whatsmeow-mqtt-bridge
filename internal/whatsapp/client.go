package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
)

type EventHandler interface {
	OnMessage(model.MessageEvent)
	OnConnected(string)
	OnDisconnected(string)
	OnLoggedOut(string)
}

type Client struct {
	client      *whatsmeow.Client
	container   *sqlstore.Container
	handler     EventHandler
	log         *slog.Logger
	qrCancel    context.CancelFunc
	mu          sync.Mutex
	eventCtx    context.Context
	eventCancel context.CancelFunc
	messages    chan model.MessageEvent
}

func (c *Client) SetEventHandler(handler EventHandler) {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
}

func Open(ctx context.Context, path string, handler EventHandler, log *slog.Logger) (*Client, error) {
	if err := os.MkdirAll(dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	container := sqlstore.NewWithDB(db, "sqlite3", nil)
	if err = container.Upgrade(ctx); err != nil {
		_ = container.Close()
		return nil, err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, err
	}
	wc := whatsmeow.NewClient(device, nil)
	wc.EnableAutoReconnect = true
	eventCtx, eventCancel := context.WithCancel(ctx)
	c := &Client{client: wc, container: container, handler: handler, log: log, eventCtx: eventCtx, eventCancel: eventCancel, messages: make(chan model.MessageEvent, 128)}
	go c.messageWorker()
	wc.AddEventHandler(c.handleEvent)
	return c, nil
}

func dir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "."
}

func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelQRLocked()
	if c.client.Store.ID == nil {
		qrCtx, cancel := context.WithCancel(ctx)
		c.qrCancel = cancel
		ch, err := c.client.GetQRChannel(qrCtx)
		if err != nil {
			cancel()
			return err
		}
		go c.consumeQR(qrCtx, ch)
	}
	return c.client.Connect()
}

func (c *Client) consumeQR(ctx context.Context, ch <-chan whatsmeow.QRChannelItem) {
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-ch:
			if !ok {
				return
			}
			switch item.Event {
			case whatsmeow.QRChannelEventCode:
				fmt.Fprintln(os.Stdout, "Scan this QR code in WhatsApp > Linked devices:")
				qrterminal.GenerateHalfBlock(item.Code, qrterminal.L, os.Stdout)
			case whatsmeow.QRChannelSuccess.Event:
				c.log.Info("WhatsApp pairing succeeded")
				return
			case whatsmeow.QRChannelTimeout.Event:
				c.log.Error("WhatsApp QR pairing timed out")
				return
			case whatsmeow.QRChannelClientOutdated.Event:
				c.log.Error("WhatsApp client is outdated")
				return
			case whatsmeow.QRChannelEventError:
				c.log.Error("WhatsApp pairing failed", "error", item.Error)
				return
			default:
				c.log.Warn("WhatsApp pairing ended", "event", item.Event)
				return
			}
		}
	}
}

func (c *Client) cancelQRLocked() {
	if c.qrCancel != nil {
		c.qrCancel()
		c.qrCancel = nil
	}
}
func (c *Client) Disconnect()                         { c.mu.Lock(); c.cancelQRLocked(); c.client.Disconnect(); c.mu.Unlock() }
func (c *Client) Reconnect(ctx context.Context) error { c.Disconnect(); return c.Connect(ctx) }
func (c *Client) Close() error                        { c.eventCancel(); c.Disconnect(); return c.container.Close() }
func (c *Client) Ready() bool                         { return c.client.IsConnected() && c.client.IsLoggedIn() }
func (c *Client) Phone() string {
	if c.client.Store.ID == nil {
		return ""
	}
	return c.client.Store.ID.User
}

var phoneRE = regexp.MustCompile(`^[0-9]{6,15}$`)

func Recipient(number string) (types.JID, error) {
	if !phoneRE.MatchString(number) {
		return types.EmptyJID, errors.New("recipient must contain 6 to 15 digits without leading +")
	}
	return types.NewJID(number, types.DefaultUserServer), nil
}

func (c *Client) SendText(ctx context.Context, to, text string) (string, time.Time, error) {
	jid, err := Recipient(to)
	if err != nil {
		return "", time.Time{}, err
	}
	resp, err := c.client.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(text)})
	return string(resp.ID), resp.Timestamp, err
}

func (c *Client) SendMedia(ctx context.Context, to, path, mime, caption, fileName string, image bool) (string, time.Time, error) {
	jid, err := Recipient(to)
	if err != nil {
		return "", time.Time{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, err
	}
	data, err := io.ReadAll(f)
	closeErr := f.Close()
	if err != nil {
		return "", time.Time{}, err
	}
	if closeErr != nil {
		return "", time.Time{}, closeErr
	}
	mediaType := whatsmeow.MediaDocument
	if image {
		mediaType = whatsmeow.MediaImage
	}
	up, err := c.client.Upload(ctx, data, mediaType)
	if err != nil {
		return "", time.Time{}, err
	}
	msg := &waE2E.Message{}
	if image {
		msg.ImageMessage = &waE2E.ImageMessage{Caption: proto.String(caption), Mimetype: proto.String(mime), URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength)}
	} else {
		msg.DocumentMessage = &waE2E.DocumentMessage{Caption: proto.String(caption), Title: proto.String(caption), FileName: proto.String(fileName), Mimetype: proto.String(mime), URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength)}
	}
	resp, err := c.client.SendMessage(ctx, jid, msg)
	return string(resp.ID), resp.Timestamp, err
}

func (c *Client) handleEvent(raw any) {
	handler := c.eventHandler()
	switch evt := raw.(type) {
	case *events.Connected:
		if handler != nil {
			handler.OnConnected(c.Phone())
		}
	case *events.Disconnected:
		if handler != nil {
			handler.OnDisconnected("WhatsApp disconnected")
		}
	case *events.LoggedOut:
		if handler != nil {
			handler.OnLoggedOut("WhatsApp session logged out")
		}
	case *events.Message:
		c.handleMessage(evt)
	}
}

func (c *Client) handleMessage(evt *events.Message) {
	if evt == nil || evt.Message == nil {
		return
	}
	info := evt.Info
	if info.IsFromMe || info.IsGroup || info.Chat.IsBroadcastList() || info.Chat.Server == types.NewsletterServer {
		return
	}
	text := evt.Message.GetConversation()
	if text == "" {
		text = evt.Message.GetExtendedTextMessage().GetText()
	}
	if text == "" {
		return
	}
	from := info.Sender.User
	if info.Sender.Server == types.HiddenUserServer {
		if !info.SenderAlt.IsEmpty() && info.SenderAlt.Server == types.DefaultUserServer {
			from = info.SenderAlt.User
		} else {
			from = info.Sender.String()
		}
	}
	evtOut := model.MessageEvent{MessageID: string(info.ID), From: from, Type: "text", Payload: map[string]any{"text": text}, Timestamp: model.UTCMillis(info.Timestamp)}
	select {
	case c.messages <- evtOut:
	case <-c.eventCtx.Done():
	}
}
