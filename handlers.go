package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/localitas/localitas-go"
	"github.com/localitas/localitas-go/httputil"
)

const unsubscribeTimeout = 15 * time.Second

type handler struct {
	app *App
}

func userID(r *http.Request) string {
	return client.GetUserID(r.Context())
}

func (h *handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.app.Store.ListAccounts(r.Context(), userID(r))
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	for _, a := range accounts {
		if a.UnreadCount, err = h.app.Store.GetAccountUnreadCount(r.Context(), a.ID); err != nil {
			writeErr(w, r, http.StatusInternalServerError, "count unread for account %s: %v", a.ID, err)
			return
		}
	}
	writeJSON(w, r, http.StatusOK, accounts)
}

func (h *handler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name              string `json:"name"`
		Email             string `json:"email"`
		Provider          string `json:"provider"`
		IMAPHost          string `json:"imap_host"`
		IMAPPort          int    `json:"imap_port"`
		SMTPHost          string `json:"smtp_host"`
		SMTPPort          int    `json:"smtp_port"`
		Username          string `json:"username"`
		Password          string `json:"password"`
		OAuthClientID     string `json:"oauth_client_id"`
		OAuthClientSecret string `json:"oauth_client_secret"`
		UseTLS            bool   `json:"use_tls"`
		VaultCredentialID string `json:"vault_credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}

	var seededRefreshToken string
	if req.VaultCredentialID != "" && h.app.client != nil {
		secrets, err := h.app.client.WithToken(client.TokenFromRequest(r)).VaultGetSecrets(r.Context(), req.VaultCredentialID)
		if err != nil {
			writeErr(w, r, http.StatusBadRequest, "failed to resolve vault credential: %v", err)
			return
		}
		if req.Email == "" {
			req.Email = secrets["email"]
		}
		if req.Username == "" {
			if secrets["username"] != "" {
				req.Username = secrets["username"]
			} else {
				req.Username = secrets["email"]
			}
		}
		if req.Password == "" {
			if secrets["email_app_password"] != "" {
				req.Password = secrets["email_app_password"]
			} else {
				req.Password = secrets["password"]
			}
		}
		if req.IMAPHost == "" {
			req.IMAPHost = secrets["imap_host"]
		}
		if req.SMTPHost == "" {
			req.SMTPHost = secrets["smtp_host"]
		}
		if req.OAuthClientID == "" {
			req.OAuthClientID = secrets["oauth_client_id"]
		}
		if req.OAuthClientSecret == "" {
			req.OAuthClientSecret = secrets["oauth_client_secret"]
		}
		// A vault credential may carry a pre-provisioned refresh_token (dev seed /
		// headless provisioning). Persist it below so the account is usable without
		// the interactive OAuth "Connect" flow after a data wipe.
		seededRefreshToken = secrets["refresh_token"]
	}

	if req.Email == "" {
		writeErr(w, r, http.StatusBadRequest, "email is required")
		return
	}
	if req.Provider == "gmail-oauth" {
		if req.OAuthClientID == "" || req.OAuthClientSecret == "" {
			writeErr(w, r, http.StatusBadRequest, "oauth_client_id and oauth_client_secret are required for Gmail OAuth")
			return
		}
		req.IMAPHost = "imap.gmail.com"
		req.IMAPPort = 993
		req.SMTPHost = "smtp.gmail.com"
		req.SMTPPort = 587
		req.UseTLS = true
		req.Username = req.Email
	} else {
		if req.IMAPHost == "" || req.Username == "" || req.Password == "" {
			writeErr(w, r, http.StatusBadRequest, "imap_host, username, password are required")
			return
		}
	}
	if req.IMAPPort == 0 {
		req.IMAPPort = 993
	}
	if req.SMTPPort == 0 {
		req.SMTPPort = 587
	}
	if req.Name == "" {
		req.Name = req.Email
	}
	if req.Provider == "" {
		req.Provider = "imap"
	}
	account, err := h.app.Store.CreateAccount(r.Context(), userID(r), req.Name, req.Email, req.Provider, req.IMAPHost, req.IMAPPort, req.SMTPHost, req.SMTPPort, req.Username, req.Password, req.OAuthClientID, req.OAuthClientSecret, req.UseTLS, req.VaultCredentialID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	// Persist a pre-provisioned refresh token (from the vault credential) so the
	// account works immediately without the interactive OAuth flow. expiresIn=0
	// forces a refresh on first use, which mints a fresh access token.
	if seededRefreshToken != "" {
		if err := h.app.Store.SaveOAuthTokens(r.Context(), account.UserID, account.ID, "", seededRefreshToken, 0); err != nil {
			writeErr(w, r, http.StatusInternalServerError, "account created but failed to save seeded token: %v", err)
			return
		}
	}
	writeJSON(w, r, http.StatusCreated, account)
}

func (h *handler) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name              string `json:"name"`
		Email             string `json:"email"`
		Provider          string `json:"provider"`
		IMAPHost          string `json:"imap_host"`
		IMAPPort          int    `json:"imap_port"`
		SMTPHost          string `json:"smtp_host"`
		SMTPPort          int    `json:"smtp_port"`
		Username          string `json:"username"`
		Password          string `json:"password"`
		OAuthClientID     string `json:"oauth_client_id"`
		OAuthClientSecret string `json:"oauth_client_secret"`
		UseTLS            bool   `json:"use_tls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.app.Store.UpdateAccount(r.Context(), userID(r), id, req.Name, req.Email, req.Provider, req.IMAPHost, req.IMAPPort, req.SMTPHost, req.SMTPPort, req.Username, req.Password, req.OAuthClientID, req.OAuthClientSecret, req.UseTLS); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Store.DeleteAccount(r.Context(), userID(r), r.PathValue("id")); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleListFolders(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		writeErr(w, r, http.StatusBadRequest, "account_id is required")
		return
	}
	if err := h.app.Store.RequireAccount(r.Context(), userID(r), accountID); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	folders, err := h.app.Store.ListFolders(r.Context(), userID(r), accountID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, folders)
}

func (h *handler) handleListEmails(w http.ResponseWriter, r *http.Request) {
	folderID := r.URL.Query().Get("folder_id")
	if folderID == "" {
		writeErr(w, r, http.StatusBadRequest, "folder_id is required")
		return
	}
	if _, err := h.app.Store.GetFolderByID(r.Context(), userID(r), folderID); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	emails, err := h.app.Store.ListEmails(r.Context(), userID(r), folderID, intParam(r, "limit", 50), intParam(r, "offset", 0))
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, emails)
}

func (h *handler) handleGetEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := h.app.Store.GetEmail(r.Context(), userID(r), id)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if err := h.app.Store.MarkRead(r.Context(), userID(r), id); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, e)
}

func (h *handler) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Store.MarkUnread(r.Context(), userID(r), r.PathValue("id")); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleToggleStar(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Store.ToggleStar(r.Context(), userID(r), r.PathValue("id")); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeErr(w, r, http.StatusBadRequest, "q is required")
		return
	}
	var emails []*Email
	var err error
	if r.URL.Query().Get("trash") == "1" {
		emails, err = h.app.Store.SearchDeletedEmails(r.Context(), userID(r), q, 20)
	} else {
		emails, err = h.app.Store.SearchEmails(r.Context(), userID(r), q, 20)
	}
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, emails)
}

// loadAccount returns userID's account id, with its OAuth tokens when it
// signs in with OAuth.
func (h *handler) loadAccount(ctx context.Context, userID, id string) (*Account, error) {
	account, err := h.app.Store.GetAccount(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !account.NeedsOAuth() {
		return account, nil
	}
	return h.app.Store.GetAccountWithTokens(ctx, userID, id)
}

func (h *handler) syncConfig() *SyncConfig {
	return &SyncConfig{CoreURL: h.app.CoreURL, AuthToken: h.app.AuthToken}
}

// recordSync stores the outcome of syncing an account; a failure to record
// it is logged, since the sync itself has already happened.
func (h *handler) recordSync(ctx context.Context, accountID string, syncErr error) {
	msg := ""
	if syncErr != nil {
		msg = syncErr.Error()
	}
	if err := h.app.Store.UpdateSyncStatus(ctx, accountID, msg); err != nil {
		logger.Error("failed to record sync status", "account_id", accountID, "error", err)
	}
}

func (h *handler) handleSync(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	account, err := h.loadAccount(r.Context(), userID(r), accountID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	result, err := SyncAccount(r.Context(), h.app.Store, account, 200, h.syncConfig())
	h.recordSync(r.Context(), accountID, err)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "sync failed: %v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *handler) handleSyncFolder(w http.ResponseWriter, r *http.Request) {
	folder, err := h.app.Store.GetFolderByID(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	account, err := h.loadAccount(r.Context(), userID(r), folder.AccountID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	result, err := SyncSingleFolder(r.Context(), h.app.Store, account, folder, 200, h.syncConfig())
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "sync failed: %v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *handler) handleDeleteEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	accountID, folderID, uid, err := h.app.Store.GetEmailUID(ctx, userID(r), id)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	account, err := h.loadAccount(ctx, userID(r), accountID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if err := EnsureValidToken(ctx, h.app.Store, account); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "oauth: %v", err)
		return
	}
	folderRemote, err := h.app.Store.GetFolderRemoteName(ctx, accountID, folderID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "folder of email %s: %v", id, err)
		return
	}
	if err := MoveToTrash(account, folderRemote, uid); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "IMAP delete failed: %v", err)
		return
	}
	if err := h.app.Store.SoftDelete(ctx, userID(r), id); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleRestoreEmail(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Store.RestoreEmail(r.Context(), userID(r), r.PathValue("id")); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleListTrash(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID != "" {
		if err := h.app.Store.RequireAccount(r.Context(), userID(r), accountID); err != nil {
			writeStoreErr(w, r, err)
			return
		}
	}
	emails, err := h.app.Store.ListDeletedEmails(r.Context(), userID(r), accountID, intParam(r, "limit", 50), intParam(r, "offset", 0))
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, emails)
}

func (h *handler) handleCompose(w http.ResponseWriter, r *http.Request) {
	var req ComposeRequest
	var attachFiles []AttachmentFile

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, r, http.StatusBadRequest, "invalid form: %v", err)
			return
		}
		req.AccountID = r.FormValue("account_id")
		req.To = splitComma(r.FormValue("to"))
		req.Cc = splitComma(r.FormValue("cc"))
		req.Bcc = splitComma(r.FormValue("bcc"))
		req.Subject = r.FormValue("subject")
		req.Body = r.FormValue("body")
		req.BodyHTML = r.FormValue("body_html")
		req.InReplyTo = r.FormValue("in_reply_to")

		for _, fh := range r.MultipartForm.File["attachments"] {
			f, err := fh.Open()
			if err != nil {
				writeErr(w, r, http.StatusBadRequest, "attachment %s: %v", fh.Filename, err)
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				writeErr(w, r, http.StatusBadRequest, "attachment %s: %v", fh.Filename, err)
				return
			}
			attachFiles = append(attachFiles, AttachmentFile{
				Filename:    fh.Filename,
				ContentType: fh.Header.Get("Content-Type"),
				Data:        data,
			})
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, r, http.StatusBadRequest, "invalid body")
			return
		}
	}

	if req.AccountID == "" || len(req.To) == 0 || req.Subject == "" {
		writeErr(w, r, http.StatusBadRequest, "account_id, to, subject are required")
		return
	}

	account, err := h.loadAccount(r.Context(), userID(r), req.AccountID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if err := EnsureValidToken(r.Context(), h.app.Store, account); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "oauth: %v", err)
		return
	}
	if err := SendEmailWithAttachments(account, req.To, req.Cc, req.Bcc, req.Subject, req.Body, req.BodyHTML, req.InReplyTo, attachFiles); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "send failed: %v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) oauthRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host + h.app.BasePath + "api/oauth/callback"
}

func (h *handler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	account, err := h.app.Store.GetAccount(r.Context(), userID(r), accountID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if account.OAuthClientID == "" {
		writeErr(w, r, http.StatusBadRequest, "no OAuth client ID configured")
		return
	}
	state, err := sealOAuthState(userID(r), accountID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "seal oauth state: %v", err)
		return
	}
	redirectURI := h.oauthRedirectURI(r)
	authURL := GoogleAuthRedirectURL(account.OAuthClientID, redirectURI, state)
	writeJSON(w, r, http.StatusOK, map[string]string{"auth_url": authURL, "redirect_uri": redirectURI})
}

// handleOAuthCallback completes a flow handleOAuthStart began. The state
// names the user who started it and the account it authorizes; the callback
// is refused unless the user completing it is that user and still owns the
// account, so tokens can only land on the caller's own account.
func (h *handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeErr(w, r, http.StatusBadRequest, "missing code or state")
		return
	}
	stateUser, accountID, err := openOAuthState(state)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "%v", err)
		return
	}
	if stateUser != userID(r) {
		writeStoreErr(w, r, ErrNotFound)
		return
	}
	// Load WITH tokens: when Google omits refresh_token on re-consent, the
	// stored one is kept instead of being overwritten with "".
	account, err := h.app.Store.GetAccountWithTokens(r.Context(), userID(r), accountID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	tok, err := ExchangeGoogleCode(r.Context(), code, account.OAuthClientID, account.OAuthClientSecret, h.oauthRedirectURI(r))
	if err != nil {
		writeOAuthPage(w, "Authorization Failed", err.Error(), 3000)
		return
	}
	refreshToken := tok.RefreshToken
	if refreshToken == "" {
		refreshToken = account.RefreshToken
	}
	if err := h.app.Store.SaveOAuthTokens(r.Context(), userID(r), accountID, tok.AccessToken, refreshToken, tok.ExpiresIn); err != nil {
		writeOAuthPage(w, "Authorization Failed", "could not save tokens: "+err.Error(), 3000)
		return
	}
	writeOAuthPage(w, "Gmail Connected!", "You can close this window.", 2000)
}

func (h *handler) handleGetThread(w http.ResponseWriter, r *http.Request) {
	threadID := r.URL.Query().Get("thread_id")
	if threadID == "" {
		writeErr(w, r, http.StatusBadRequest, "thread_id is required")
		return
	}
	emails, err := h.app.Store.ListThreadEmails(r.Context(), userID(r), threadID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	if len(emails) == 0 {
		writeStoreErr(w, r, ErrNotFound)
		return
	}
	writeJSON(w, r, http.StatusOK, emails)
}

func (h *handler) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	emailID := r.PathValue("id")
	if _, err := h.app.Store.GetEmail(r.Context(), userID(r), emailID); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	attachments, err := h.app.Store.ListAttachments(r.Context(), userID(r), emailID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, attachments)
}

func (h *handler) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
	att, err := h.app.Store.GetAttachment(r.Context(), userID(r), r.PathValue("aid"))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	fileID, err := h.app.attachmentFileID(r.Context(), att)
	if err != nil {
		status := http.StatusBadGateway
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		writeErr(w, r, status, "download failed: %v", err)
		return
	}
	// The bytes are read before any header is sent, so a failure is
	// answered as one rather than as a truncated download.
	var buf bytes.Buffer
	if err := DownloadAttachment(r.Context(), h.app.CoreURL, h.app.AuthToken, fileID, &buf); err != nil {
		status := http.StatusBadGateway
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		writeErr(w, r, status, "download failed: %v", err)
		return
	}
	contentType := att.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": att.Filename}))
	w.Write(buf.Bytes())
}

// attachmentFileID returns the managed file of att. One stored by path
// before IDs is resolved once and its ID recorded.
func (a *App) attachmentFileID(ctx context.Context, att *Attachment) (string, error) {
	if att.FileID != "" {
		return att.FileID, nil
	}
	if att.StoragePath == "" {
		return "", fmt.Errorf("attachment %s has no stored file", att.ID)
	}
	file, err := ResolveAttachmentPath(ctx, a.CoreURL, a.AuthToken, att.StoragePath)
	if err != nil {
		return "", err
	}
	if err := a.Store.SetAttachmentFile(ctx, att.ID, file.ID); err != nil {
		return "", err
	}
	att.FileID, att.StoragePath = file.ID, ""
	return file.ID, nil
}

func (h *handler) handleSaveDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID           string `json:"id"`
		AccountID    string `json:"account_id"`
		ToAddresses  string `json:"to_addresses"`
		CcAddresses  string `json:"cc_addresses"`
		BccAddresses string `json:"bcc_addresses"`
		Subject      string `json:"subject"`
		BodyText     string `json:"body_text"`
		BodyHTML     string `json:"body_html"`
		InReplyTo    string `json:"in_reply_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AccountID == "" {
		writeErr(w, r, http.StatusBadRequest, "account_id is required")
		return
	}
	draft, err := h.app.Store.SaveDraft(r.Context(), userID(r), req.ID, req.AccountID, req.ToAddresses, req.CcAddresses, req.BccAddresses, req.Subject, req.BodyText, req.BodyHTML, req.InReplyTo)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, draft)
}

func (h *handler) handleListDrafts(w http.ResponseWriter, r *http.Request) {
	drafts, err := h.app.Store.ListDrafts(r.Context(), userID(r))
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, drafts)
}

func (h *handler) handleGetDraft(w http.ResponseWriter, r *http.Request) {
	draft, err := h.app.Store.GetDraft(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, draft)
}

func (h *handler) handleDeleteDraft(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Store.DeleteDraft(r.Context(), userID(r), r.PathValue("id")); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleListPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, Presets)
}

// handleUnsubscribe unsubscribes from the sender of the caller's email using
// only the List-Unsubscribe target recorded when the email was synced. The
// request body is never read: a caller-supplied URL would let any user make
// the server fetch an arbitrary URL, or send mail from their account to an
// arbitrary address.
func (h *handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	e, err := h.app.Store.GetEmail(ctx, userID(r), id)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	target := e.UnsubscribeURL
	isHTTP := strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")
	isMailto := strings.HasPrefix(target, "mailto:")
	if !isHTTP && !isMailto {
		writeErr(w, r, http.StatusBadRequest, "no unsubscribe URL available")
		return
	}

	if isHTTP {
		err = unsubscribeHTTP(ctx, target, e.UnsubscribePost)
	} else {
		err = h.unsubscribeMailto(ctx, userID(r), e.AccountID, strings.TrimPrefix(target, "mailto:"))
	}
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "unsubscribe failed: %v", err)
		return
	}
	if err := h.app.Store.SoftDelete(ctx, userID(r), id); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func unsubscribeHTTP(ctx context.Context, target string, oneClickPost bool) error {
	ctx, cancel := context.WithTimeout(ctx, unsubscribeTimeout)
	defer cancel()
	var req *http.Request
	var err error
	if oneClickPost {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader("List-Unsubscribe=One-Click-Unsubscribe"))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	}
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// unsubscribeMailto sends the unsubscribe request from the account the
// email arrived in, which the caller owns.
func (h *handler) unsubscribeMailto(ctx context.Context, userID, accountID, addr string) error {
	account, err := h.loadAccount(ctx, userID, accountID)
	if err != nil {
		return fmt.Errorf("load account: %w", err)
	}
	if err := EnsureValidToken(ctx, h.app.Store, account); err != nil {
		return fmt.Errorf("oauth: %w", err)
	}
	return SendEmailWithAttachments(account, []string{addr}, nil, nil, "Unsubscribe", "Unsubscribe", "", "", nil)
}

// SyncAllResult is what one run of the sync job reports.
type SyncAllResult struct {
	AccountsSynced int      `json:"accounts_synced"`
	NewEmails      int      `json:"new_emails"`
	Errors         []string `json:"errors"`
}

// doSyncAll syncs every user's accounts. Each account is loaded with its
// secrets as its owner, so password and OAuth accounts both sign in.
func (h *handler) doSyncAll(ctx context.Context) (*SyncAllResult, error) {
	accounts, err := h.app.Store.ListAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := &SyncAllResult{AccountsSynced: len(accounts), Errors: make([]string, 0)}
	for _, a := range accounts {
		account, err := h.loadAccount(ctx, a.UserID, a.ID)
		if err != nil {
			result.Errors = append(result.Errors, a.Name+": "+err.Error())
			continue
		}
		synced, err := SyncAccount(ctx, h.app.Store, account, 200, h.syncConfig())
		h.recordSync(ctx, a.ID, err)
		if err != nil {
			result.Errors = append(result.Errors, a.Name+": "+err.Error())
			continue
		}
		result.NewEmails += synced.NewEmails
	}
	return result, nil
}

// handleSyncAll is the sync job automation runs every five minutes; it acts
// across every user's accounts, so only core may call it.
func (h *handler) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	work := func(ctx context.Context) (map[string]interface{}, error) {
		result, err := h.doSyncAll(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"accounts_synced": result.AccountsSynced,
			"new_emails":      result.NewEmails,
			"errors":          result.Errors,
		}, nil
	}
	if h.app.client.RunAsync(w, r, work) {
		return
	}
	result, err := h.doSyncAll(r.Context())
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *handler) handleListFilters(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		writeErr(w, r, http.StatusBadRequest, "account_id is required")
		return
	}
	if err := h.app.Store.RequireAccount(r.Context(), userID(r), accountID); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	filters, err := h.app.Store.ListFilters(r.Context(), userID(r), accountID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, filters)
}

func (h *handler) handleCreateFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID string `json:"account_id"`
		Rule      string `json:"rule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AccountID == "" || req.Rule == "" {
		writeErr(w, r, http.StatusBadRequest, "account_id and rule are required")
		return
	}
	filter, err := h.app.Store.CreateFilter(r.Context(), userID(r), req.AccountID, req.Rule)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, filter)
}

func (h *handler) handleUpdateFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rule     string `json:"rule"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Rule == "" {
		writeErr(w, r, http.StatusBadRequest, "rule is required")
		return
	}
	if err := h.app.Store.UpdateFilter(r.Context(), userID(r), r.PathValue("id"), req.Rule, req.IsActive); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Store.DeleteFilter(r.Context(), userID(r), r.PathValue("id")); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleTestFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rule string `json:"rule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	pf, err := ParseFilter(req.Rule)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, pf)
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v interface{}) {
	httputil.WriteResponse(w, r, status, v)
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, format string, args ...interface{}) {
	httputil.WriteError(w, r, status, format, args...)
}

// writeStoreErr answers 404 for a record that is not the caller's (or does
// not exist), 400 for an invalid filter rule, and 500 with the real cause for
// anything else, e.g. a secret that can't be decrypted.
func writeStoreErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, r, http.StatusNotFound, "not found")
	case errors.Is(err, ErrInvalidFilter):
		writeErr(w, r, http.StatusBadRequest, "%v", err)
	default:
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
	}
}

// writeOAuthPage renders the small popup page the OAuth flow lands on.
func writeOAuthPage(w http.ResponseWriter, title, message string, closeAfterMs int) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body><h3>%s</h3><p>%s</p><script>setTimeout(function(){window.close();},%d);</script></body></html>`,
		html.EscapeString(title), html.EscapeString(message), closeAfterMs)
}

func intParam(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
