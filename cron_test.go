package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleCron(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var spec struct {
		Jobs []struct {
			ID       string `json:"id"`
			Path     string `json:"path"`
			Method   string `json:"method"`
			Schedule string `json:"schedule"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(spec.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(spec.Jobs))
	}

	job := spec.Jobs[0]
	if job.ID != "cron:email:sync-all" {
		t.Errorf("expected id cron:email:sync-all, got %s", job.ID)
	}
	if job.Path != "/api/sync-all" {
		t.Errorf("expected path /api/sync-all, got %s", job.Path)
	}
	if job.Schedule != "*/5 * * * *" {
		t.Errorf("expected schedule */5 * * * *, got %s", job.Schedule)
	}
}

func TestHandleCron_ContentType(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestHandleCron_RetryConfig(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	var spec struct {
		Jobs []struct {
			Retry struct {
				MaxAttempts int `json:"max_attempts"`
			} `json:"retry"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if spec.Jobs[0].Retry.MaxAttempts != 1 {
		t.Errorf("expected max_attempts 1, got %d", spec.Jobs[0].Retry.MaxAttempts)
	}
}

func TestHandleCron_ScheduleFormat(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	var spec struct {
		Jobs []struct {
			Schedule string `json:"schedule"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for i, job := range spec.Jobs {
		fields := strings.Fields(job.Schedule)
		if len(fields) != 5 {
			t.Errorf("job[%d]: expected 5 cron fields, got %d in %q", i, len(fields), job.Schedule)
		}
	}
}
