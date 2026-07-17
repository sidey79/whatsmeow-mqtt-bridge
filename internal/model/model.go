package model

import (
	"encoding/json"
	"time"
)

const (
	StateStarting     = "starting"
	StateRegistered   = "registered"
	StateConnecting   = "connecting"
	StateReady        = "ready"
	StateDisconnected = "disconnected"
	StateError        = "error"
)

const (
	ErrorInvalidPayload       = "INVALID_PAYLOAD"
	ErrorNotReady             = "NOT_READY"
	ErrorSendFailed           = "SEND_FAILED"
	ErrorMediaDownloadFailed  = "MEDIA_DOWNLOAD_FAILED"
	ErrorMediaTooLarge        = "MEDIA_TOO_LARGE"
	ErrorMediaHostNotAllowed  = "MEDIA_HOST_NOT_ALLOWED"
	ErrorSessionInvalid       = "SESSION_INVALID"
	ErrorWhatsAppDisconnected = "WHATSAPP_DISCONNECTED"
)

type CommandEnvelope struct {
	RequestID string          `json:"requestId"`
	To        string          `json:"to"`
	Payload   json.RawMessage `json:"payload"`
}

type TextPayload struct {
	Text string `json:"text"`
}

type ImagePayload struct {
	URL     string `json:"url"`
	Caption string `json:"caption,omitempty"`
}

type DocumentPayload struct {
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	FileName string `json:"fileName,omitempty"`
}

type StatusEvent struct {
	State     string `json:"state"`
	Connected bool   `json:"connected"`
	Phone     string `json:"phone"`
	Message   string `json:"message,omitempty"`
}

type DeliveryEvent struct {
	RequestID string    `json:"requestId"`
	Status    string    `json:"status"`
	MessageID string    `json:"messageId"`
	Timestamp time.Time `json:"timestamp"`
}

type ErrorEvent struct {
	RequestID string    `json:"requestId"`
	Status    string    `json:"status"`
	ErrorCode string    `json:"errorCode"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type MessageEvent struct {
	MessageID string         `json:"messageId"`
	From      string         `json:"from"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
}

func UTCMillis(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }
