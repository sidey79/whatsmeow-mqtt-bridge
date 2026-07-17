package mqttbridge

import "strings"

type Topics struct{ base string }

func NewTopics(base string) Topics          { return Topics{base: strings.Trim(base, "/")} }
func (t Topics) Command(kind string) string { return t.base + "/cmd/" + kind }
func (t Topics) Event(kind string) string   { return t.base + "/event/" + kind }
func (t Topics) Commands() []string {
	return []string{t.Command("send/text"), t.Command("send/image"), t.Command("send/document"), t.Command("status"), t.Command("reconnect")}
}

func (t Topics) CommandKind(topic string) (string, bool) {
	prefix := t.base + "/cmd/"
	if !strings.HasPrefix(topic, prefix) {
		return "", false
	}
	kind := strings.TrimPrefix(topic, prefix)
	switch kind {
	case "send/text", "send/image", "send/document", "status", "reconnect":
		return kind, true
	}
	return "", false
}
