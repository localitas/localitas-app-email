package email

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
)

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
			log.Printf("install: attempt %d failed (retrying): %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := applyEmbeddedMigrations(ctx, a.client, db.ID); err != nil {
			log.Printf("install: migrations attempt %d failed (retrying): %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		return db.ID, nil
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/index.html")
	if err != nil {
		log.Printf("email index template error: %v", err)
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
	mux.HandleFunc("GET /api/accounts", h.handleListAccounts)
	mux.HandleFunc("POST /api/accounts", h.handleCreateAccount)
	mux.HandleFunc("PUT /api/accounts/{id}", h.handleUpdateAccount)
	mux.HandleFunc("DELETE /api/accounts/{id}", h.handleDeleteAccount)
	mux.HandleFunc("POST /api/accounts/{id}/sync", h.handleSync)
	mux.HandleFunc("POST /api/folders/{id}/sync", h.handleSyncFolder)
	mux.HandleFunc("GET /api/folders", h.handleListFolders)
	mux.HandleFunc("GET /api/emails", h.handleListEmails)
	mux.HandleFunc("GET /api/thread", h.handleGetThread)
	mux.HandleFunc("GET /api/emails/{id}", h.handleGetEmail)
	mux.HandleFunc("POST /api/emails/{id}/star", h.handleToggleStar)
	mux.HandleFunc("POST /api/emails/{id}/unsubscribe", h.handleUnsubscribe)
	mux.HandleFunc("POST /api/emails/{id}/unread", h.handleMarkUnread)
	mux.HandleFunc("DELETE /api/emails/{id}", h.handleDeleteEmail)
	mux.HandleFunc("POST /api/emails/{id}/restore", h.handleRestoreEmail)
	mux.HandleFunc("GET /api/trash", h.handleListTrash)
	mux.HandleFunc("POST /api/compose", h.handleCompose)
	mux.HandleFunc("GET /api/search", h.handleSearch)
	mux.HandleFunc("POST /api/drafts", h.handleSaveDraft)
	mux.HandleFunc("GET /api/drafts", h.handleListDrafts)
	mux.HandleFunc("GET /api/drafts/{id}", h.handleGetDraft)
	mux.HandleFunc("DELETE /api/drafts/{id}", h.handleDeleteDraft)
	mux.HandleFunc("GET /api/emails/{id}/attachments", h.handleListAttachments)
	mux.HandleFunc("GET /api/attachments/{aid}/download", h.handleDownloadAttachment)
	mux.HandleFunc("GET /api/oauth/{id}/start", h.handleOAuthStart)
	mux.HandleFunc("GET /api/oauth/callback", h.handleOAuthCallback)
	mux.HandleFunc("POST /api/sync-all", h.handleSyncAll)
	mux.HandleFunc("GET /api/presets", h.handleListPresets)
	mux.HandleFunc("GET /api/filters", h.handleListFilters)
	mux.HandleFunc("POST /api/filters", h.handleCreateFilter)
	mux.HandleFunc("PUT /api/filters/{id}", h.handleUpdateFilter)
	mux.HandleFunc("DELETE /api/filters/{id}", h.handleDeleteFilter)
	mux.HandleFunc("POST /api/filters/test", h.handleTestFilter)
}
