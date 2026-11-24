package email

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexRendersBasePathAndDocs(t *testing.T) {
	a := New(nil, "/apps/email/")
	rec := httptest.NewRecorder()
	a.handleIndex(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `<base href="/apps/email/">`) {
		t.Errorf("base href missing from index")
	}
	if !strings.Contains(body, string(RenderDocsHTML(EmailAPIDoc))) {
		t.Errorf("docs HTML missing from index")
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
		t.Errorf("index did not render to its end")
	}
}
