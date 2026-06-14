package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSwagger(t *testing.T) {
	w := httptest.NewRecorder()
	HandleSwagger(w, httptest.NewRequest("GET", "/swagger.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var spec APIDoc
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if spec.AppName != "Email" {
		t.Errorf("expected app_name Email, got %q", spec.AppName)
	}
	expected := []string{"/api/accounts", "/api/folders", "/api/emails", "/api/search", "/api/presets", "/api/trash", "/api/compose"}
	for _, p := range expected {
		found := false
		for _, ep := range spec.Endpoints {
			if strings.Contains(ep.Path, p) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected endpoint containing %s", p)
		}
	}
}

func TestPresets(t *testing.T) {
	if len(Presets) == 0 {
		t.Error("expected at least one provider preset")
	}
	gmail := Presets[0]
	if gmail.Name != "Gmail (OAuth)" {
		t.Errorf("expected first preset to be Gmail (OAuth), got %s", gmail.Name)
	}
	if gmail.IMAPHost != "imap.gmail.com" {
		t.Errorf("expected imap.gmail.com, got %s", gmail.IMAPHost)
	}
	if gmail.IMAPPort != 993 {
		t.Errorf("expected port 993, got %d", gmail.IMAPPort)
	}
}

func TestHandleListPresets(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	h.handleListPresets(w, httptest.NewRequest("GET", "/api/presets", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var presets []ProviderPreset
	if err := json.Unmarshal(w.Body.Bytes(), &presets); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(presets) != len(Presets) {
		t.Errorf("expected %d presets, got %d", len(Presets), len(presets))
	}
}

func TestHandleListEmails_MissingFolderID(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	h.handleListEmails(w, httptest.NewRequest("GET", "/api/emails", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleListFolders_MissingAccountID(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	h.handleListFolders(w, httptest.NewRequest("GET", "/api/folders", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSearch_MissingQuery(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	h.handleSearch(w, httptest.NewRequest("GET", "/api/search", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateAccount_MissingFields(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	body := `{"name":"test"}`
	h.handleCreateAccount(w, httptest.NewRequest("POST", "/api/accounts", strings.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateAccount_InvalidBody(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	h.handleCreateAccount(w, httptest.NewRequest("POST", "/api/accounts", strings.NewReader("not json")))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCompose_MissingFields(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	body := `{"account_id":"abc"}`
	h.handleCompose(w, httptest.NewRequest("POST", "/api/compose", strings.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCompose_InvalidBody(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	h.handleCompose(w, httptest.NewRequest("POST", "/api/compose", strings.NewReader("bad")))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestShouldSyncFolder(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"INBOX", true},
		{"inbox", true},
		{"Sent", true},
		{"Sent Messages", true},
		{"Draft", true},
		{"Drafts", true},
		{"Starred", true},
		{"Important", true},
		{"Trash", false},
		{"Spam", false},
		{"Junk", false},
		{"[Gmail]/All Mail", false},
	}
	for _, tt := range tests {
		if got := shouldSyncFolder(tt.name); got != tt.want {
			t.Errorf("shouldSyncFolder(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIntParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?limit=25&bad=xyz", nil)
	if got := intParam(req, "limit", 50); got != 25 {
		t.Errorf("intParam(limit) = %d, want 25", got)
	}
	if got := intParam(req, "bad", 50); got != 50 {
		t.Errorf("intParam(bad) = %d, want 50 (default)", got)
	}
	if got := intParam(req, "missing", 10); got != 10 {
		t.Errorf("intParam(missing) = %d, want 10 (default)", got)
	}
}

func TestHandleSwagger_AllEndpoints(t *testing.T) {
	w := httptest.NewRecorder()
	HandleSwagger(w, httptest.NewRequest("GET", "/swagger.json", nil))
	var spec APIDoc
	json.Unmarshal(w.Body.Bytes(), &spec)

	expected := []string{"/api/emails/{id}", "/api/compose", "/api/trash"}
	for _, p := range expected {
		found := false
		for _, ep := range spec.Endpoints {
			if strings.Contains(ep.Path, p) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected endpoint containing %s", p)
		}
	}
}

func TestHandleListTrash_NoStore(t *testing.T) {
	h := &handler{app: &App{}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/trash", nil)
	defer func() {
		if rec := recover(); rec != nil {
			t.Log("panicked as expected with nil store")
		}
	}()
	h.handleListTrash(w, r)
}

func TestComposeRequest_Validation(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{"empty body", `{}`, http.StatusBadRequest},
		{"missing to", `{"account_id":"a","subject":"hi"}`, http.StatusBadRequest},
		{"missing subject", `{"account_id":"a","to":["b@c.com"]}`, http.StatusBadRequest},
		{"missing account", `{"to":["b@c.com"],"subject":"hi"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &handler{app: &App{}}
			w := httptest.NewRecorder()
			h.handleCompose(w, httptest.NewRequest("POST", "/api/compose", strings.NewReader(tt.body)))
			if w.Code != tt.code {
				t.Errorf("expected %d, got %d", tt.code, w.Code)
			}
		})
	}
}

func TestEmailAPIDoc_EndpointCount(t *testing.T) {
	if len(EmailAPIDoc.Endpoints) < 14 {
		t.Errorf("expected at least 14 endpoints, got %d", len(EmailAPIDoc.Endpoints))
	}
}

func TestNewID(t *testing.T) {
	id1 := newID()
	id2 := newID()
	if id1 == "" {
		t.Error("newID returned empty string")
	}
	if len(id1) != 32 {
		t.Errorf("newID length = %d, want 32 hex chars", len(id1))
	}
	if id1 == id2 {
		t.Error("two newID calls should produce different IDs")
	}
}
