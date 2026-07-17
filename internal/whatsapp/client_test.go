package whatsapp

import "testing"

func TestRecipient(t *testing.T) {
	for _, valid := range []string{"123456", "491701234567", "123456789012345"} {
		jid, err := Recipient(valid)
		if err != nil || jid.User != valid {
			t.Fatalf("%q: %v %v", valid, jid, err)
		}
	}
	for _, bad := range []string{"+491701234567", "12345", "1234567890123456", "12a456", ""} {
		if _, err := Recipient(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
