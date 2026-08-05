package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	MQTTURL             string
	MQTTUsername        string
	MQTTPassword        string
	MQTTBaseTopic       string
	MQTTClientID        string
	MQTTProtocolVersion int
	WADBPath            string
	MediaAllowedHosts   []string
	MediaMaxBytes       int64
	LogLevel            string
	HealthPort          int
	Database            DatabaseConfig
}

func Load() (Config, error) { return FromLookup(os.LookupEnv) }

func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(k, d string) string {
		if v, ok := lookup(k); ok {
			return strings.TrimSpace(v)
		}
		return d
	}
	mqttURL := get("MQTT_URL", "")
	if mqttURL == "" {
		host := get("MQTT_HOST", "mqtt")
		port := get("MQTT_PORT", "1883")
		mqttURL = "mqtt://" + net.JoinHostPort(host, port)
	}
	u, err := url.Parse(mqttURL)
	if err != nil || (u.Scheme != "mqtt" && u.Scheme != "mqtts" && u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		return Config{}, fmt.Errorf("MQTT_URL must be a mqtt, mqtts, ws, or wss URL")
	}
	protocolVersion, err := strconv.Atoi(get("MQTT_PROTOCOL_VERSION", "3"))
	if err != nil || (protocolVersion != 3 && protocolVersion != 5) {
		return Config{}, fmt.Errorf("MQTT_PROTOCOL_VERSION must be 3 or 5")
	}
	base, err := NormalizeBaseTopic(get("MQTT_BASE_TOPIC", "whatsmeow-mqtt-bridge"))
	if err != nil {
		return Config{}, err
	}
	maxMB, err := strconv.ParseInt(get("MEDIA_MAX_SIZE_MB", "10"), 10, 64)
	if err != nil || maxMB < 1 || maxMB > 1024 {
		return Config{}, fmt.Errorf("MEDIA_MAX_SIZE_MB must be between 1 and 1024")
	}
	port, err := strconv.Atoi(get("HEALTH_PORT", "3000"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("HEALTH_PORT must be between 1 and 65535")
	}
	level := strings.ToLower(get("LOG_LEVEL", "info"))
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		return Config{}, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
	user := get("MQTT_USERNAME", "")
	if user == "" {
		user = get("MQTT_USER", "")
	}
	var hosts []string
	for _, h := range strings.Split(get("MEDIA_ALLOWED_HOSTS", ""), ",") {
		if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
			hosts = append(hosts, h)
		}
	}
	database, err := databaseFromLookup(lookup)
	if err != nil {
		return Config{}, err
	}
	return Config{MQTTURL: mqttURL, MQTTUsername: user, MQTTPassword: get("MQTT_PASSWORD", ""), MQTTBaseTopic: base,
		MQTTClientID: get("MQTT_CLIENT_ID", "whatsmeow-mqtt-bridge"), MQTTProtocolVersion: protocolVersion, WADBPath: get("WA_DB_PATH", "/data/whatsapp_session.db"),
		MediaAllowedHosts: hosts, MediaMaxBytes: maxMB * 1024 * 1024, LogLevel: level, HealthPort: port, Database: database}, nil
}

func NormalizeBaseTopic(s string) (string, error) {
	s = strings.Trim(strings.TrimSpace(s), "/")
	if s == "" || strings.ContainsAny(s, "#+\x00") || strings.Contains(s, "//") {
		return "", fmt.Errorf("MQTT_BASE_TOPIC is invalid")
	}
	return s, nil
}
