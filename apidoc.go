package email

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

type APIEndpoint struct {
	Method      string     `json:"method"`
	Path        string     `json:"path"`
	Summary     string     `json:"summary"`
	QueryParams []APIParam `json:"query_params,omitempty"`
	RequestBody *APIBody   `json:"request_body,omitempty"`
	Response    *APIBody   `json:"response,omitempty"`
}

type APIParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type APIBody struct {
	ContentType string `json:"content_type"`
	Example     string `json:"example"`
}

type APIDoc struct {
	AppName     string        `json:"app_name"`
	Version     string        `json:"version"`
	Description string        `json:"description"`
	Keywords    []string      `json:"keywords,omitempty"`
	Endpoints   []APIEndpoint `json:"endpoints"`
}

var EmailAPIDoc = APIDoc{
	AppName:     "Email",
	Version:     "0.1.0",
	Description: "IMAP email client with account management, folder sync, and full-text search",
	Keywords:    []string{"email", "mail", "inbox", "message", "send", "reply", "forward", "attachment", "IMAP", "folder", "unread", "draft", "spam", "compose"},
	Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/accounts", Summary: "List email accounts", Response: &APIBody{ContentType: "application/json", Example: `[{"id":"abc...","name":"Personal","email":"user@gmail.com","imap_host":"imap.gmail.com"}]`}},
		{Method: "POST", Path: "/api/accounts", Summary: "Add an email account", RequestBody: &APIBody{ContentType: "application/json", Example: `{"name":"Work","email":"me@company.com","imap_host":"imap.gmail.com","imap_port":993,"smtp_host":"smtp.gmail.com","smtp_port":587,"username":"me@company.com","password":"app-password","use_tls":true}`}, Response: &APIBody{ContentType: "application/json", Example: `{"id":"abc...","name":"Work","email":"me@company.com"}`}},
		{Method: "DELETE", Path: "/api/accounts/{id}", Summary: "Remove an account and all its emails", Response: &APIBody{ContentType: "application/json", Example: `{"success":true}`}},
		{Method: "GET", Path: "/api/folders", Summary: "List folders for an account", QueryParams: []APIParam{{Name: "account_id", Type: "string", Required: true, Description: "Account ID"}}, Response: &APIBody{ContentType: "application/json", Example: `[{"id":"abc...","name":"INBOX","unread_count":5,"total_count":120}]`}},
		{Method: "GET", Path: "/api/emails", Summary: "List emails in a folder", QueryParams: []APIParam{{Name: "folder_id", Type: "string", Required: true, Description: "Folder ID"}, {Name: "limit", Type: "integer", Description: "Max results"}, {Name: "offset", Type: "integer", Description: "Offset"}}, Response: &APIBody{ContentType: "application/json", Example: `[{"id":"abc...","subject":"Hello","from_name":"Alice","snippet":"Hi there..."}]`}},
		{Method: "GET", Path: "/api/emails/{id}", Summary: "Get full email with body (marks as read)", Response: &APIBody{ContentType: "application/json", Example: `{"id":"abc...","subject":"Hello","body_text":"Full message...","body_html":"<p>Full message</p>"}`}},
		{Method: "POST", Path: "/api/emails/{id}/star", Summary: "Toggle email star", Response: &APIBody{ContentType: "application/json", Example: `{"success":true}`}},
		{Method: "POST", Path: "/api/emails/{id}/unread", Summary: "Mark email as unread", Response: &APIBody{ContentType: "application/json", Example: `{"success":true}`}},
		{Method: "DELETE", Path: "/api/emails/{id}", Summary: "Soft delete an email (move to trash)", Response: &APIBody{ContentType: "application/json", Example: `{"success":true}`}},
		{Method: "POST", Path: "/api/emails/{id}/restore", Summary: "Restore a soft-deleted email from trash", Response: &APIBody{ContentType: "application/json", Example: `{"success":true}`}},
		{Method: "GET", Path: "/api/trash", Summary: "List soft-deleted emails (trash)", QueryParams: []APIParam{{Name: "account_id", Type: "string", Description: "Filter by account"}, {Name: "limit", Type: "integer", Description: "Max results"}, {Name: "offset", Type: "integer", Description: "Offset"}}, Response: &APIBody{ContentType: "application/json", Example: `[{"id":"abc...","subject":"Deleted email","from_name":"Alice"}]`}},
		{Method: "POST", Path: "/api/accounts/{id}/sync", Summary: "Sync account (fetch new emails via IMAP)", Response: &APIBody{ContentType: "application/json", Example: `{"folders":5,"new_emails":12,"duration_ms":3400}`}},
		{Method: "GET", Path: "/api/search", Summary: "Search emails (FTS5)", QueryParams: []APIParam{{Name: "q", Type: "string", Required: true, Description: "Search query"}}, Response: &APIBody{ContentType: "application/json", Example: `[{"id":"abc...","subject":"Meeting","from_name":"Bob"}]`}},
		{Method: "POST", Path: "/api/compose", Summary: "Send an email via SMTP", RequestBody: &APIBody{ContentType: "application/json", Example: `{"account_id":"abc...","to":["bob@example.com"],"subject":"Hello","body":"Hi there"}`}, Response: &APIBody{ContentType: "application/json", Example: `{"success":true}`}},
		{Method: "GET", Path: "/api/presets", Summary: "List IMAP/SMTP presets for common providers", Response: &APIBody{ContentType: "application/json", Example: `[{"name":"Gmail","imap_host":"imap.gmail.com","imap_port":993}]`}},
	},
}

func HandleSwagger(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(EmailAPIDoc)
}

func RenderDocsHTML(doc APIDoc) template.HTML {
	var sb strings.Builder
	sb.WriteString(`<h3 style="font-size: 0.875rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--color-text-secondary); margin-bottom: 1rem;">API Endpoints</h3><div class="accordion-list">`)
	for _, ep := range doc.Endpoints {
		title := fmt.Sprintf("%s %s — %s", ep.Method, ep.Path, ep.Summary)
		sb.WriteString(fmt.Sprintf(`<details class="glass-panel" style="border-radius: 0.5rem; margin-bottom: 0.5rem;"><summary style="padding: 0.75rem 1rem; cursor: pointer; font-weight: 500; color: var(--color-text-primary);">%s</summary><div style="padding: 0 1rem 0.75rem; font-size: 0.875rem; color: var(--color-text-secondary);">`, template.HTMLEscapeString(title)))
		var ex strings.Builder
		if ep.RequestBody != nil {
			ex.WriteString("# Request\n")
			ex.WriteString(prettyJSON(ep.RequestBody.Example))
			ex.WriteString("\n\n")
		}
		if ep.Response != nil {
			ex.WriteString("# Response\n")
			ex.WriteString(prettyJSON(ep.Response.Example))
		}
		if ex.Len() > 0 {
			sb.WriteString(fmt.Sprintf(`<pre style="background: var(--color-bg-base); padding: 0.75rem; border-radius: 0.375rem; overflow-x: auto; font-size: 0.8125rem;">%s</pre>`, template.HTMLEscapeString(ex.String())))
		}
		sb.WriteString(`</div></details>`)
	}
	sb.WriteString(`</div>`)
	return template.HTML(sb.String())
}

func prettyJSON(s string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
