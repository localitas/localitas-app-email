package email

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/localitas/localitas-go"
)

type handler struct {
	app *App
}

func (h *handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	userID := client.UserIDFromRequest(r)
	accounts, err := h.app.Store.ListAccounts(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	for _, a := range accounts {
		a.UnreadCount = h.app.Store.GetAccountUnreadCount(r.Context(), a.ID)
	}
	writeJSON(w, http.StatusOK, accounts)
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
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	if req.VaultCredentialID != "" && h.app.client != nil {
		secrets, err := h.app.client.WithToken(client.TokenFromRequest(r)).VaultGetSecrets(r.Context(), req.VaultCredentialID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "failed to resolve vault credential: %v", err)
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
	}

	if req.Email == "" {
		writeErr(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Provider == "gmail-oauth" {
		if req.OAuthClientID == "" || req.OAuthClientSecret == "" {
			writeErr(w, http.StatusBadRequest, "oauth_client_id and oauth_client_secret are required for Gmail OAuth")
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
			writeErr(w, http.StatusBadRequest, "imap_host, username, password are required")
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
	userID := client.UserIDFromRequest(r)
	account, err := h.app.Store.CreateAccount(r.Context(), userID, req.Name, req.Email, req.Provider, req.IMAPHost, req.IMAPPort, req.SMTPHost, req.SMTPPort, req.Username, req.Password, req.OAuthClientID, req.OAuthClientSecret, req.UseTLS, req.VaultCredentialID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusCreated, account)
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
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.app.Store.UpdateAccount(r.Context(), id, req.Name, req.Email, req.Provider, req.IMAPHost, req.IMAPPort, req.SMTPHost, req.SMTPPort, req.Username, req.Password, req.OAuthClientID, req.OAuthClientSecret, req.UseTLS); err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.app.Store.DeleteAccount(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleListFolders(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		writeErr(w, http.StatusBadRequest, "account_id is required")
		return
	}
	folders, err := h.app.Store.ListFolders(r.Context(), accountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, folders)
}

func (h *handler) handleListEmails(w http.ResponseWriter, r *http.Request) {
	folderID := r.URL.Query().Get("folder_id")
	if folderID == "" {
		writeErr(w, http.StatusBadRequest, "folder_id is required")
		return
	}
	limit := intParam(r, "limit", 50)
	offset := intParam(r, "offset", 0)
	emails, err := h.app.Store.ListEmails(r.Context(), folderID, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

func (h *handler) handleGetEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := h.app.Store.GetEmail(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "email not found")
		return
	}
	h.app.Store.MarkRead(r.Context(), id)
	writeJSON(w, http.StatusOK, e)
}

func (h *handler) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.app.Store.MarkUnread(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleToggleStar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.app.Store.ToggleStar(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeErr(w, http.StatusBadRequest, "q is required")
		return
	}
	var emails []*Email
	var err error
	if r.URL.Query().Get("trash") == "1" {
		emails, err = h.app.Store.SearchDeletedEmails(r.Context(), q, 20)
	} else {
		emails, err = h.app.Store.SearchEmails(r.Context(), q, 20)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

func (h *handler) handleSync(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	var account *Account
	var err error
	account, err = h.app.Store.GetAccount(r.Context(), accountID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	if account.NeedsOAuth() {
		account, err = h.app.Store.GetAccountWithTokens(r.Context(), accountID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "get tokens: %v", err)
			return
		}
	}
	result, err := SyncAccount(r.Context(), h.app.Store, account, 200, &SyncConfig{CoreURL: h.app.CoreURL, AuthToken: h.app.AuthToken})
	if err != nil {
		h.app.Store.UpdateSyncStatus(r.Context(), accountID, err.Error())
		writeErr(w, http.StatusInternalServerError, "sync failed: %v", err)
		return
	}
	h.app.Store.UpdateSyncStatus(r.Context(), accountID, "")
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) handleSyncFolder(w http.ResponseWriter, r *http.Request) {
	folderID := r.PathValue("id")
	folder, err := h.app.Store.GetFolderByID(r.Context(), folderID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "folder not found")
		return
	}
	var account *Account
	account, err = h.app.Store.GetAccount(r.Context(), folder.AccountID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	if account.NeedsOAuth() {
		account, err = h.app.Store.GetAccountWithTokens(r.Context(), folder.AccountID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "get tokens: %v", err)
			return
		}
	}
	result, err := SyncSingleFolder(r.Context(), h.app.Store, account, folder, 200, &SyncConfig{CoreURL: h.app.CoreURL, AuthToken: h.app.AuthToken})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "sync failed: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) handleDeleteEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	accountID, folderID, uid, err := h.app.Store.GetEmailUID(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "email not found")
		return
	}

	account, err := h.app.Store.GetAccount(ctx, accountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "account not found: %v", err)
		return
	}
	if account.NeedsOAuth() {
		account, err = h.app.Store.GetAccountWithTokens(ctx, accountID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "get tokens: %v", err)
			return
		}
		if err := EnsureValidToken(ctx, h.app.Store, account); err != nil {
			writeErr(w, http.StatusInternalServerError, "oauth: %v", err)
			return
		}
	}

	folderRemote, err := h.app.Store.GetFolderRemoteName(ctx, folderID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "folder not found: %v", err)
		return
	}

	if err := MoveToTrash(account, folderRemote, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "IMAP delete failed: %v", err)
		return
	}

	if err := h.app.Store.SoftDelete(ctx, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleRestoreEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.app.Store.RestoreEmail(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleListTrash(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	limit := intParam(r, "limit", 50)
	offset := intParam(r, "offset", 0)
	emails, err := h.app.Store.ListDeletedEmails(r.Context(), accountID, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

func (h *handler) handleCompose(w http.ResponseWriter, r *http.Request) {
	var req ComposeRequest
	var attachFiles []AttachmentFile

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		r.ParseMultipartForm(32 << 20)
		req.AccountID = r.FormValue("account_id")
		req.To = splitComma(r.FormValue("to"))
		req.Cc = splitComma(r.FormValue("cc"))
		req.Bcc = splitComma(r.FormValue("bcc"))
		req.Subject = r.FormValue("subject")
		req.Body = r.FormValue("body")
		req.BodyHTML = r.FormValue("body_html")
		req.InReplyTo = r.FormValue("in_reply_to")

		files := r.MultipartForm.File["attachments"]
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(f)
			f.Close()
			attachFiles = append(attachFiles, AttachmentFile{
				Filename:    fh.Filename,
				ContentType: fh.Header.Get("Content-Type"),
				Data:        data,
			})
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
	}

	if req.AccountID == "" || len(req.To) == 0 || req.Subject == "" {
		writeErr(w, http.StatusBadRequest, "account_id, to, subject are required")
		return
	}

	var account *Account
	var err error
	account, err = h.app.Store.GetAccount(r.Context(), req.AccountID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	if account.NeedsOAuth() {
		account, err = h.app.Store.GetAccountWithTokens(r.Context(), req.AccountID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "get tokens: %v", err)
			return
		}
		if err := EnsureValidToken(r.Context(), h.app.Store, account); err != nil {
			writeErr(w, http.StatusInternalServerError, "oauth: %v", err)
			return
		}
	}

	if err := SendEmailWithAttachments(account, req.To, req.Cc, req.Bcc, req.Subject, req.Body, req.BodyHTML, req.InReplyTo, attachFiles); err != nil {
		writeErr(w, http.StatusInternalServerError, "send failed: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
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
	account, err := h.app.Store.GetAccount(r.Context(), accountID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	if account.OAuthClientID == "" {
		writeErr(w, http.StatusBadRequest, "no OAuth client ID configured")
		return
	}
	redirectURI := h.oauthRedirectURI(r)
	authURL := GoogleAuthRedirectURL(account.OAuthClientID, redirectURI, accountID)
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": authURL, "redirect_uri": redirectURI})
}

func (h *handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	accountID := r.URL.Query().Get("state")
	if code == "" || accountID == "" {
		writeErr(w, http.StatusBadRequest, "missing code or state")
		return
	}
	account, err := h.app.Store.GetAccount(r.Context(), accountID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	redirectURI := h.oauthRedirectURI(r)
	tok, err := ExchangeGoogleCode(r.Context(), code, account.OAuthClientID, account.OAuthClientSecret, redirectURI)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><h3>Authorization Failed</h3><p>%s</p><script>setTimeout(function(){window.close();},3000);</script></body></html>`, err.Error())
		return
	}
	refreshToken := tok.RefreshToken
	if refreshToken == "" {
		refreshToken = account.RefreshToken
	}
	h.app.Store.SaveOAuthTokens(r.Context(), accountID, tok.AccessToken, refreshToken, tok.ExpiresIn)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body><h3>Gmail Connected!</h3><p>You can close this window.</p><script>setTimeout(function(){window.close();},2000);</script></body></html>`)
}

func (h *handler) handleGetThread(w http.ResponseWriter, r *http.Request) {
	threadID := r.URL.Query().Get("thread_id")
	if threadID == "" {
		writeErr(w, http.StatusBadRequest, "thread_id is required")
		return
	}
	emails, err := h.app.Store.ListThreadEmails(r.Context(), threadID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

func (h *handler) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	emailID := r.PathValue("id")
	attachments, err := h.app.Store.ListAttachments(r.Context(), emailID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, attachments)
}

func (h *handler) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
	attachID := r.PathValue("aid")
	var att Attachment
	err := h.app.Store.db.QueryRowContext(r.Context(), "SELECT id, email_id, account_id, filename, content_type, size_bytes, storage_path, created_at FROM attachments WHERE id = ?", attachID).
		Scan(&att.ID, &att.EmailID, &att.AccountID, &att.Filename, &att.ContentType, &att.SizeBytes, &att.StoragePath, &att.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "attachment not found")
		return
	}
	data, contentType, err := GetAttachmentFromManagedFS(r.Context(), h.app.CoreURL, h.app.AuthToken, att.StoragePath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "download failed: %v", err)
		return
	}
	if contentType == "" {
		contentType = att.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+att.Filename+"\"")
	w.Write(data)
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
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	draft, err := h.app.Store.SaveDraft(r.Context(), req.ID, req.AccountID, req.ToAddresses, req.CcAddresses, req.BccAddresses, req.Subject, req.BodyText, req.BodyHTML, req.InReplyTo)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *handler) handleListDrafts(w http.ResponseWriter, r *http.Request) {
	drafts, err := h.app.Store.ListDrafts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, drafts)
}

func (h *handler) handleGetDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	draft, err := h.app.Store.GetDraft(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "draft not found")
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *handler) handleDeleteDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.app.Store.DeleteDraft(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleListPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Presets)
}

func (h *handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var req struct {
		URL string `json:"url"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	e, err := h.app.Store.GetEmail(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "email not found")
		return
	}
	if e.UnsubscribeURL == "" && req.URL != "" {
		e.UnsubscribeURL = req.URL
	}
	if e.UnsubscribeURL == "" {
		writeErr(w, http.StatusBadRequest, "no unsubscribe URL available")
		return
	}

	unsubErr := ""
	if e.UnsubscribePost && strings.HasPrefix(e.UnsubscribeURL, "http") {
		req, _ := http.NewRequestWithContext(ctx, "POST", e.UnsubscribeURL, strings.NewReader("List-Unsubscribe=One-Click-Unsubscribe"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			unsubErr = err.Error()
		} else {
			resp.Body.Close()
		}
	} else if strings.HasPrefix(e.UnsubscribeURL, "http") {
		req, _ := http.NewRequestWithContext(ctx, "GET", e.UnsubscribeURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			unsubErr = err.Error()
		} else {
			resp.Body.Close()
		}
	} else if strings.HasPrefix(e.UnsubscribeURL, "mailto:") {
		addr := strings.TrimPrefix(e.UnsubscribeURL, "mailto:")
		account, err := h.app.Store.GetAccount(ctx, e.AccountID)
		if err == nil {
			if account.NeedsOAuth() {
				account, _ = h.app.Store.GetAccountWithTokens(ctx, e.AccountID)
				EnsureValidToken(ctx, h.app.Store, account)
			}
			sendErr := SendEmailWithAttachments(account, []string{addr}, nil, nil, "Unsubscribe", "Unsubscribe", "", "", nil)
			if sendErr != nil {
				unsubErr = sendErr.Error()
			}
		}
	}

	if unsubErr != "" {
		writeErr(w, http.StatusInternalServerError, "unsubscribe failed: %s", unsubErr)
		return
	}

	h.app.Store.SoftDelete(ctx, id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	userID := client.UserIDFromRequest(r)
	var accounts []*Account
	var err error
	if userID != "" {
		accounts, err = h.app.Store.ListAccounts(r.Context(), userID)
	} else {
		accounts, err = h.app.Store.ListAllAccounts(r.Context())
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}

	totalNew := 0
	var errors []string
	for _, a := range accounts {
		account := a
		if account.NeedsOAuth() {
			account, err = h.app.Store.GetAccountWithTokens(r.Context(), a.ID)
			if err != nil {
				errors = append(errors, a.Name+": "+err.Error())
				continue
			}
		}
		result, err := SyncAccount(r.Context(), h.app.Store, account, 200, &SyncConfig{CoreURL: h.app.CoreURL, AuthToken: h.app.AuthToken})
		if err != nil {
			errors = append(errors, a.Name+": "+err.Error())
			h.app.Store.UpdateSyncStatus(r.Context(), a.ID, err.Error())
			continue
		}
		h.app.Store.UpdateSyncStatus(r.Context(), a.ID, "")
		totalNew += result.NewEmails
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"accounts_synced": len(accounts),
		"new_emails":      totalNew,
		"errors":          errors,
	})
}

func (h *handler) handleListFilters(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		writeErr(w, http.StatusBadRequest, "account_id is required")
		return
	}
	filters, err := h.app.Store.ListFilters(r.Context(), accountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if filters == nil {
		filters = make([]*Filter, 0)
	}
	writeJSON(w, http.StatusOK, filters)
}

func (h *handler) handleCreateFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID string `json:"account_id"`
		Rule      string `json:"rule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AccountID == "" || req.Rule == "" {
		writeErr(w, http.StatusBadRequest, "account_id and rule are required")
		return
	}
	filter, err := h.app.Store.CreateFilter(r.Context(), req.AccountID, req.Rule)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	writeJSON(w, http.StatusCreated, filter)
}

func (h *handler) handleUpdateFilter(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Rule     string `json:"rule"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Rule == "" {
		writeErr(w, http.StatusBadRequest, "rule is required")
		return
	}
	if err := h.app.Store.UpdateFilter(r.Context(), id, req.Rule, req.IsActive); err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.app.Store.DeleteFilter(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleTestFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rule string `json:"rule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	pf, err := ParseFilter(req.Rule)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, pf)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, format string, args ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf(format, args...)})
}

func intParam(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
