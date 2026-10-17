package email

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
)

var logger = slog.Default().With("component", "email")

type App struct {
	Store     *Store
	BasePath  string
	CoreURL   string
	AuthToken string
	client    *client.Client
}

func New(c *client.Client, basePath string) *App {
	if basePath == "" {
		basePath = "/"
	}
	return &App{BasePath: basePath, client: c}
}

func (a *App) InitStore(coreURL, dbID, token string) error {
	store, err := OpenStore(coreURL, dbID, token)
	if err != nil {
		return err
	}
	a.Store = store
	return nil
}

func (a *App) Install(ctx context.Context) (string, error) {
	for attempt := 1; ; attempt++ {
		db, err := a.client.CreateSystemDatabase(ctx, DatabaseName)
		if err != nil {
			logger.Warn("install attempt failed", "attempt", attempt, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := applyEmbeddedMigrations(ctx, a.client, db.ID); err != nil {
			logger.Warn("install migrations attempt failed", "attempt", attempt, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		return db.ID, nil
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/index.html")
	if err != nil {
		logger.Error("index template error", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	tmpl.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"BasePath": a.BasePath,
		"DocsHTML": RenderDocsHTML(EmailAPIDoc),
	})
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	h := &handler{app: a}

	mux.HandleFunc("GET /{$}", a.handleIndex)
	mux.HandleFunc("GET /swagger.json", HandleSwagger)
	mux.HandleFunc("GET /help.md", handleHelpMarkdown)

	// Every API route acts for the user core vouches for; one without a
	// verified user is refused, and changing anything needs write scope.
	user := func(pattern string, handler http.HandlerFunc) {
		mux.Handle(pattern, client.AuthMiddleware(handler))
	}
	writer := func(pattern string, handler http.HandlerFunc) {
		user(pattern, client.RequireScopeFunc(client.ScopeWrite, handler))
	}

	user("GET /api/accounts", h.handleListAccounts)
	writer("POST /api/accounts", h.handleCreateAccount)
	writer("PUT /api/accounts/{id}", h.handleUpdateAccount)
	writer("DELETE /api/accounts/{id}", h.handleDeleteAccount)
	writer("POST /api/accounts/{id}/sync", h.handleSync)
	writer("POST /api/folders/{id}/sync", h.handleSyncFolder)
	user("GET /api/folders", h.handleListFolders)
	user("GET /api/emails", h.handleListEmails)
	user("GET /api/thread", h.handleGetThread)
	user("GET /api/emails/{id}", h.handleGetEmail)
	writer("POST /api/emails/{id}/star", h.handleToggleStar)
	writer("POST /api/emails/{id}/unsubscribe", h.handleUnsubscribe)
	writer("POST /api/emails/{id}/unread", h.handleMarkUnread)
	writer("DELETE /api/emails/{id}", h.handleDeleteEmail)
	writer("POST /api/emails/{id}/restore", h.handleRestoreEmail)
	user("GET /api/trash", h.handleListTrash)
	writer("POST /api/compose", h.handleCompose)
	user("GET /api/search", h.handleSearch)
	writer("POST /api/drafts", h.handleSaveDraft)
	user("GET /api/drafts", h.handleListDrafts)
	user("GET /api/drafts/{id}", h.handleGetDraft)
	writer("DELETE /api/drafts/{id}", h.handleDeleteDraft)
	user("GET /api/emails/{id}/attachments", h.handleListAttachments)
	user("GET /api/attachments/{aid}/download", h.handleDownloadAttachment)
	writer("GET /api/oauth/{id}/start", h.handleOAuthStart)
	writer("GET /api/oauth/callback", h.handleOAuthCallback)
	user("GET /api/presets", h.handleListPresets)
	user("GET /api/filters", h.handleListFilters)
	writer("POST /api/filters", h.handleCreateFilter)
	writer("PUT /api/filters/{id}", h.handleUpdateFilter)
	writer("DELETE /api/filters/{id}", h.handleDeleteFilter)
	user("POST /api/filters/test", h.handleTestFilter)

	// The sync job runs across every user's accounts: only core's own
	// scheduled call may reach it.
	mux.HandleFunc("POST /api/sync-all", client.RequireSystem(h.handleSyncAll))
}
