package config

import "testing"

func lookup(values map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := values[k]; return v, ok }
}

func TestDefaults(t *testing.T) {
	c, err := FromLookup(lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.MQTTURL != "mqtt://mqtt:1883" || c.MQTTBaseTopic != "whatsmeow-mqtt-bridge" || c.MQTTProtocolVersion != 3 || c.WADBPath != "/data/whatsapp_session.db" || c.MediaMaxBytes != 10*1024*1024 || c.HealthPort != 3000 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestAliasesAndNormalization(t *testing.T) {
	c, err := FromLookup(lookup(map[string]string{"MQTT_HOST": "broker", "MQTT_PORT": "2883", "MQTT_USER": "old", "MQTT_BASE_TOPIC": "/legacy/base/"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.MQTTURL != "mqtt://broker:2883" || c.MQTTUsername != "old" || c.MQTTBaseTopic != "legacy/base" {
		t.Fatalf("unexpected: %+v", c)
	}
}

func TestValidation(t *testing.T) {
	for name, values := range map[string]map[string]string{"url": {"MQTT_URL": "http://bad"}, "topic": {"MQTT_BASE_TOPIC": "bad/#"}, "size": {"MEDIA_MAX_SIZE_MB": "0"}, "port": {"HEALTH_PORT": "70000"}, "level": {"LOG_LEVEL": "trace"}, "protocol": {"MQTT_PROTOCOL_VERSION": "4"}} {
		t.Run(name, func(t *testing.T) {
			if _, err := FromLookup(lookup(values)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
