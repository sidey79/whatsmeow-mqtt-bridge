package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)
import (
	"github.com/sven/whatsmeow-mqtt-bridge/internal/bridge"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/config"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/health"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/media"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/model"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/mqttbridge"
	"github.com/sven/whatsmeow-mqtt-bridge/internal/whatsapp"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		port := os.Getenv("HEALTH_PORT")
		if port == "" {
			port = "3000"
		}
		resp, err := http.Get("http://127.0.0.1:" + port + "/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = resp.Body.Close()
		return
	}
	if err := run(); err != nil {
		slog.Error("bridge stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	wa, err := whatsapp.Open(ctx, whatsapp.StoreConfig{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN}, nil, log)
	if err != nil {
		return fmt.Errorf("open WhatsApp store: %w", err)
	}
	defer wa.Close()
	var app *bridge.Bridge
	mqtt, err := mqttbridge.New(ctx, mqttbridge.Config{URL: cfg.MQTTURL, Username: cfg.MQTTUsername, Password: cfg.MQTTPassword, ClientID: cfg.MQTTClientID, BaseTopic: cfg.MQTTBaseTopic, ProtocolVersion: cfg.MQTTProtocolVersion, OnConnectionUp: func() {
		if app != nil {
			app.PublishCurrentStatus(context.Background())
		}
	}}, func(cmdCtx context.Context, kind string, payload []byte) {
		if app != nil {
			app.HandleCommand(cmdCtx, kind, payload)
		}
	}, log)
	if err != nil {
		return fmt.Errorf("start MQTT: %w", err)
	}
	app = bridge.New(mqtt, wa, media.NewDownloader(cfg.MediaAllowedHosts, cfg.MediaMaxBytes), log)
	wa.SetEventHandler(app)
	hs := health.New(cfg.HealthPort, app)
	go func() {
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("health server stopped", "error", err)
			stop()
		}
	}()
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = mqtt.WaitForConnection(connectCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("MQTT initial connection: %w", err)
	}
	app.SetConnecting(ctx)
	if err = wa.Connect(ctx); err != nil {
		return fmt.Errorf("WhatsApp connect: %w", err)
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = mqtt.PublishStatus(shutdownCtx, model.StatusEvent{State: model.StateDisconnected, Connected: false, Phone: wa.Phone(), Message: "Bridge shutting down"})
	wa.Disconnect()
	_ = hs.Shutdown(shutdownCtx)
	return mqtt.Close(shutdownCtx)
}
