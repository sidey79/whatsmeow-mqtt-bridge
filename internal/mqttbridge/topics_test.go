package mqttbridge

import "testing"

func TestTopicsPreserveContract(t *testing.T) {
	topics := NewTopics("whalibmob")
	want := []string{"whalibmob/cmd/send/text", "whalibmob/cmd/send/image", "whalibmob/cmd/send/document", "whalibmob/cmd/status", "whalibmob/cmd/reconnect"}
	got := topics.Commands()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("topic %d: %q", i, got[i])
		}
		if kind, ok := topics.CommandKind(got[i]); !ok || kind == "" {
			t.Fatalf("route %q", got[i])
		}
	}
	if topics.Event("status") != "whalibmob/event/status" {
		t.Fatal("status topic changed")
	}
	if _, ok := topics.CommandKind("other/cmd/status"); ok {
		t.Fatal("routed foreign topic")
	}
}
