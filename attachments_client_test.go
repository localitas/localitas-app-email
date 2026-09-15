package email

import (
	"testing"
	"time"
)

// Attachment save/get and the antivirus scan used to allocate a fresh
// http.Client per attachment inside the IMAP sync loop. They now share one
// pooled client and bound each call with a context deadline. Assert the shared
// client exists and the per-operation timeouts are sane.
func TestSharedAttachmentHTTPClient(t *testing.T) {
	if httpClient == nil {
		t.Fatal("attachment/scan calls must share a pooled http client")
	}
	for name, d := range map[string]time.Duration{
		"attachmentIO":   attachmentIOTimeout,
		"attachmentScan": attachmentScanTimeout,
		"oauth":          oauthTimeout,
	} {
		if d <= 0 || d > 2*time.Minute {
			t.Errorf("%s timeout = %v, want a sane positive bound", name, d)
		}
	}
}
