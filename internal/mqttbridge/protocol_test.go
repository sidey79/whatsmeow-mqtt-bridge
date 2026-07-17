package mqttbridge

import "testing"

func TestMQTTV3URL(t *testing.T) {
	for input, want := range map[string]string{
		"mqtt://broker:1883":  "tcp://broker:1883",
		"mqtts://broker:8883": "ssl://broker:8883",
		"ws://broker/mqtt":    "ws://broker/mqtt",
		"wss://broker/mqtt":   "wss://broker/mqtt",
	} {
		got, err := mqttV3URL(input)
		if err != nil || got != want {
			t.Fatalf("mqttV3URL(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := mqttV3URL("http://broker"); err == nil {
		t.Fatal("HTTP URL accepted")
	}
}
