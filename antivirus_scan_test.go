package email

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// antivirusScanResponse is the scan result antivirus answers POST
// /api/scan-file with (localitas-app-antivirus ScanResult).
type antivirusScanResponse struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	FileSize int64  `json:"file_size"`
	Verdict  string `json:"verdict"`
}

// antivirusThroughCore stands in for antivirus behind core: core serves an
// external app at /apps/ext/<app>/ and hands it the rest of the path, and
// antivirus scans the multipart "file" of POST /api/scan-file. Bytes holding
// "EICAR" are infected. Every other path is 404, as core answers it.
func antivirusThroughCore(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /apps/ext/antivirus/api/scan-file", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer email-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file is required", http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(f)
		verdict := "clean"
		if strings.Contains(string(data), "EICAR") {
			verdict = "infected"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(antivirusScanResponse{ID: "scan-1", Filename: h.Filename, FileSize: int64(len(data)), Verdict: verdict})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// An attachment is scanned at the route antivirus serves, and its verdict is
// the one antivirus gives.
func TestScanAttachment_UsesTheScanFileRoute(t *testing.T) {
	srv := antivirusThroughCore(t)
	cfg := &SyncConfig{CoreURL: srv.URL, AuthToken: "email-token"}

	if v, err := scanAttachment(context.Background(), cfg, attachmentData{Filename: "report.pdf", Data: []byte("quarterly")}); v != scanClean || err != nil {
		t.Errorf("a clean attachment: %v %v, want clean", v, err)
	}
	if v, err := scanAttachment(context.Background(), cfg, attachmentData{Filename: "eicar.com", Data: []byte("X5O!P%@AP EICAR test")}); v != scanInfected || err != nil {
		t.Errorf("an infected attachment: %v %v, want infected", v, err)
	}
}

// A scan that fails or gives no verdict leaves the attachment not scanned,
// never clean.
func TestScanAttachment_FailedScanIsNotClean(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "scan failed: clamd is not running", http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)
	noVerdict := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"scan-1"}`))
	}))
	t.Cleanup(noVerdict.Close)
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()

	for name, coreURL := range map[string]string{
		"antivirus fails":       failing.URL,
		"antivirus no verdict":  noVerdict.URL,
		"core unreachable":      gone.URL,
		"antivirus not serving": antivirusThroughCore(t).URL + "/elsewhere",
	} {
		v, err := scanAttachment(context.Background(), &SyncConfig{CoreURL: coreURL, AuthToken: "email-token"}, attachmentData{Filename: "report.pdf", Data: []byte("quarterly")})
		if v != scanUnscanned || err == nil {
			t.Errorf("%s: %v %v, want not scanned with a reason", name, v, err)
		}
	}
}

// An email with an infected attachment is marked dangerous and the
// attachment is not kept.
func TestSaveEmailAttachments_InfectedMarksTheEmailDangerous(t *testing.T) {
	env := newTestEnv(t)
	srv := antivirusThroughCore(t)
	cfg := &SyncConfig{CoreURL: srv.URL, AuthToken: "email-token"}
	mb := env.alice
	e := env.addEmail(mb, 2, "invoice", "")

	saveEmailAttachments(context.Background(), env.store, cfg, mb.account.ID, e.MessageID, []attachmentData{{Filename: "invoice.exe", Data: []byte("EICAR")}})

	got, err := env.store.GetEmail(context.Background(), mb.user, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsDangerous {
		t.Error("an email with an infected attachment is not marked dangerous")
	}
	atts, err := env.store.ListAttachments(context.Background(), mb.user, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 0 {
		t.Errorf("the infected attachment was kept: %+v", atts)
	}
}
