package mqttbridge

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
)

type MQTTClient interface {
	WaitForConnection(context.Context) error
	Connected() bool
	Close(context.Context) error
	PublishStatus(context.Context, model.StatusEvent) error
	PublishMessage(context.Context, model.MessageEvent) error
	PublishDelivery(context.Context, model.DeliveryEvent) error
	PublishError(context.Context, model.ErrorEvent) error
}

func New(ctx context.Context, cfg Config, handler Handler, log *slog.Logger) (MQTTClient, error) {
	switch cfg.ProtocolVersion {
	case 3:
		return newV3(ctx, cfg, handler, log)
	case 5:
		return newV5(ctx, cfg, handler, log)
	default:
		return nil, fmt.Errorf("unsupported MQTT protocol version %d", cfg.ProtocolVersion)
	}
}
