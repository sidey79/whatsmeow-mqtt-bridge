package mqttbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
)

const (
	maxCommandBytes  = 1024 * 1024
	commandQueueSize = 64
	commandWorkers   = 4
)

type Handler func(context.Context, string, []byte)

type Config struct {
	URL, Username, Password, ClientID, BaseTopic string
	ProtocolVersion                              int
	OnConnectionUp                               func()
}

type command struct {
	kind    string
	payload []byte
}

type Client struct {
	cm        *autopaho.ConnectionManager
	topics    Topics
	handler   Handler
	connected atomic.Bool
	log       *slog.Logger
	ctx       context.Context
	commands  chan command
}

func newV5(ctx context.Context, cfg Config, handler Handler, log *slog.Logger) (*Client, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, err
	}
	c := &Client{topics: NewTopics(cfg.BaseTopic), handler: handler, log: log, ctx: ctx, commands: make(chan command, commandQueueSize)}
	for range commandWorkers {
		go c.commandWorker()
	}
	will, _ := json.Marshal(model.StatusEvent{State: model.StateDisconnected, Connected: false, Phone: "", Message: "MQTT connection lost"})
	cc := autopaho.ClientConfig{ServerUrls: []*url.URL{u}, KeepAlive: 30, ConnectRetryDelay: 5 * time.Second, CleanStartOnInitialConnection: false, SessionExpiryInterval: 86400,
		ConnectUsername: cfg.Username, ConnectPassword: []byte(cfg.Password), WillMessage: &paho.WillMessage{Topic: c.topics.Event("status"), Payload: will, QoS: 1, Retain: true},
		ClientConfig: paho.ClientConfig{ClientID: cfg.ClientID, OnPublishReceived: []func(paho.PublishReceived) (bool, error){c.onPublish}, OnClientError: func(err error) { c.connected.Store(false); log.Warn("MQTT connection lost", "error", err) }},
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			c.connected.Store(true)
			if err := c.subscribe(context.Background()); err != nil {
				log.Error("MQTT subscribe failed", "error", err)
				return
			}
			if cfg.OnConnectionUp != nil {
				cfg.OnConnectionUp()
			}
		},
		OnConnectError: func(err error) { c.connected.Store(false); log.Warn("MQTT connection failed", "error", err) },
	}
	cm, err := autopaho.NewConnection(ctx, cc)
	if err != nil {
		return nil, err
	}
	c.cm = cm
	return c, nil
}

func (c *Client) WaitForConnection(ctx context.Context) error { return c.cm.AwaitConnection(ctx) }
func (c *Client) Connected() bool                             { return c.connected.Load() }
func (c *Client) Close(ctx context.Context) error {
	c.connected.Store(false)
	return c.cm.Disconnect(ctx)
}
func (c *Client) subscribe(ctx context.Context) error {
	subs := make([]paho.SubscribeOptions, 0, len(c.topics.Commands()))
	for _, topic := range c.topics.Commands() {
		subs = append(subs, paho.SubscribeOptions{Topic: topic, QoS: 1})
	}
	_, err := c.cm.Subscribe(ctx, &paho.Subscribe{Subscriptions: subs})
	return err
}

func (c *Client) onPublish(pr paho.PublishReceived) (bool, error) {
	p := pr.Packet
	if p == nil {
		return false, nil
	}
	kind, ok := c.topics.CommandKind(p.Topic)
	if !ok {
		return false, nil
	}
	var payload []byte
	if len(p.Payload) > maxCommandBytes {
		c.log.Warn("MQTT command exceeds size limit", "topic", p.Topic)
	} else {
		payload = append([]byte(nil), p.Payload...)
	}
	c.enqueue(command{kind: kind, payload: payload})
	return true, nil
}
func (c *Client) enqueue(cmd command) {
	select {
	case c.commands <- cmd:
	case <-c.ctx.Done():
	}
}
func (c *Client) commandWorker() {
	for {
		select {
		case cmd := <-c.commands:
			if c.handler != nil {
				c.handler(c.ctx, cmd.kind, cmd.payload)
			}
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Client) publish(ctx context.Context, topic string, value any, retain bool) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = c.cm.Publish(ctx, &paho.Publish{Topic: topic, QoS: 1, Retain: retain, Payload: b})
	return err
}
func (c *Client) PublishStatus(ctx context.Context, v model.StatusEvent) error {
	return c.publish(ctx, c.topics.Event("status"), v, true)
}
func (c *Client) PublishMessage(ctx context.Context, v model.MessageEvent) error {
	return c.publish(ctx, c.topics.Event("message"), v, false)
}
func (c *Client) PublishDelivery(ctx context.Context, v model.DeliveryEvent) error {
	return c.publish(ctx, c.topics.Event("delivery"), v, false)
}
func (c *Client) PublishError(ctx context.Context, v model.ErrorEvent) error {
	return c.publish(ctx, c.topics.Event("error"), v, false)
}
func (c *Client) PublishLog(ctx context.Context, v any) error {
	return fmt.Errorf("publishing logs is disabled to prevent secret leakage: %v", v)
}
