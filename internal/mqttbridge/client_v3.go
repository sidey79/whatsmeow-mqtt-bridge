package mqttbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
)

type clientV3 struct {
	client       mqtt.Client
	topics       Topics
	handler      Handler
	connected    atomic.Bool
	log          *slog.Logger
	ctx          context.Context
	commands     chan command
	connectionUp chan struct{}
}

func newV3(ctx context.Context, cfg Config, handler Handler, log *slog.Logger) (*clientV3, error) {
	brokerURL, err := mqttV3URL(cfg.URL)
	if err != nil {
		return nil, err
	}
	c := &clientV3{topics: NewTopics(cfg.BaseTopic), handler: handler, log: log, ctx: ctx, commands: make(chan command, commandQueueSize), connectionUp: make(chan struct{}, 1)}
	for range commandWorkers {
		go c.commandWorker()
	}
	will, _ := json.Marshal(model.StatusEvent{State: model.StateDisconnected, Connected: false, Phone: "", Message: "MQTT connection lost"})
	opts := mqtt.NewClientOptions().AddBroker(brokerURL).SetClientID(cfg.ClientID).SetProtocolVersion(4).
		SetUsername(cfg.Username).SetPassword(cfg.Password).SetCleanSession(false).SetResumeSubs(true).
		SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(5*time.Second).
		SetMaxReconnectInterval(30*time.Second).SetKeepAlive(30*time.Second).SetPingTimeout(10*time.Second).
		SetOrderMatters(false).SetWill(c.topics.Event("status"), string(will), 1, true)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		c.connected.Store(false)
		log.Warn("MQTT connection lost", "error", err)
	})
	opts.SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) { c.connected.Store(false) })
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		filters := make(map[string]byte, len(c.topics.Commands()))
		for _, topic := range c.topics.Commands() {
			filters[topic] = 1
		}
		token := client.SubscribeMultiple(filters, c.onMessage)
		if !token.WaitTimeout(15*time.Second) || token.Error() != nil {
			log.Error("MQTT subscribe failed", "error", token.Error())
			return
		}
		c.connected.Store(true)
		select {
		case c.connectionUp <- struct{}{}:
		default:
		}
		if cfg.OnConnectionUp != nil {
			cfg.OnConnectionUp()
		}
	})
	c.client = mqtt.NewClient(opts)
	token := c.client.Connect()
	go func() {
		token.Wait()
		if err := token.Error(); err != nil && ctx.Err() == nil {
			log.Warn("MQTT connection failed", "error", err)
		}
	}()
	return c, nil
}

func mqttV3URL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "mqtt":
		u.Scheme = "tcp"
	case "mqtts":
		u.Scheme = "ssl"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported MQTT URL scheme %q", u.Scheme)
	}
	return u.String(), nil
}
func (c *clientV3) onMessage(_ mqtt.Client, msg mqtt.Message) {
	kind, ok := c.topics.CommandKind(msg.Topic())
	if !ok {
		return
	}
	payload := msg.Payload()
	if len(payload) > maxCommandBytes {
		c.log.Warn("MQTT command exceeds size limit", "topic", msg.Topic())
		payload = nil
	} else {
		payload = append([]byte(nil), payload...)
	}
	c.enqueue(command{kind: kind, payload: payload})
}
func (c *clientV3) enqueue(cmd command) {
	select {
	case c.commands <- cmd:
	case <-c.ctx.Done():
	}
}
func (c *clientV3) commandWorker() {
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
func (c *clientV3) WaitForConnection(ctx context.Context) error {
	if c.connected.Load() {
		return nil
	}
	select {
	case <-c.connectionUp:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *clientV3) Connected() bool { return c.connected.Load() }
func (c *clientV3) Close(context.Context) error {
	c.connected.Store(false)
	if c.client.IsConnected() {
		c.client.Disconnect(250)
	}
	return nil
}
func (c *clientV3) publish(ctx context.Context, topic string, value any, retain bool) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	token := c.client.Publish(topic, 1, retain, b)
	for {
		if token.WaitTimeout(50 * time.Millisecond) {
			return token.Error()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}
func (c *clientV3) PublishStatus(ctx context.Context, v model.StatusEvent) error {
	return c.publish(ctx, c.topics.Event("status"), v, true)
}
func (c *clientV3) PublishMessage(ctx context.Context, v model.MessageEvent) error {
	return c.publish(ctx, c.topics.Event("message"), v, false)
}
func (c *clientV3) PublishDelivery(ctx context.Context, v model.DeliveryEvent) error {
	return c.publish(ctx, c.topics.Event("delivery"), v, false)
}
func (c *clientV3) PublishError(ctx context.Context, v model.ErrorEvent) error {
	return c.publish(ctx, c.topics.Event("error"), v, false)
}

var _ MQTTClient = (*clientV3)(nil)
