package email

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/localitas/localitas-go"
)

const DatabaseName = "email"

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func OpenStore(coreURL, dbID, token string) (*Store, error) {
	dsn := fmt.Sprintf("%s?database_id=%s&token=%s", coreURL, dbID, token)
	db, err := sql.Open("localitas", dsn)
	if err != nil {
		return nil, err
	}
	return NewStore(db), nil
}

func (s *Store) Close() error { return s.db.Close() }

// Accounts

func (s *Store) CreateAccount(ctx context.Context, userID, name, email, provider, imapHost string, imapPort int, smtpHost string, smtpPort int, username, password, oauthClientID, oauthClientSecret string, useTLS bool, vaultCredentialID string) (*Account, error) {
	id := newID()
	now := time.Now().UTC().Unix()
	tls := 0
	if useTLS {
		tls = 1
	}
	if provider == "" {
		provider = "imap"
	}
	encPass, _ := client.Encrypt(password)
	encSecret, _ := client.Encrypt(oauthClientSecret)
	_, err := s.db.ExecContext(ctx, `INSERT INTO accounts (id, user_id, name, email, provider, imap_host, imap_port, smtp_host, smtp_port, username, password, oauth_client_id, oauth_client_secret, use_tls, vault_credential_id, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, userID, name, email, provider, imapHost, imapPort, smtpHost, smtpPort, username, encPass, oauthClientID, encSecret, tls, vaultCredentialID, now, now)
	if err != nil {
		return nil, err
	}
	return &Account{ID: id, Name: name, Email: email, Provider: provider, IMAPHost: imapHost, IMAPPort: imapPort, SMTPHost: smtpHost, SMTPPort: smtpPort, Username: username, OAuthClientID: oauthClientID, VaultCredentialID: vaultCredentialID, UseTLS: useTLS, IsActive: true, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (s *Store) ListAccounts(ctx context.Context, userID string) ([]*Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, email, COALESCE(provider,'imap'), imap_host, imap_port, smtp_host, smtp_port, username, use_tls, COALESCE(oauth_client_id,''), COALESCE(vault_credential_id,''), last_synced_at, COALESCE(sync_error,''), is_active, created_at, updated_at FROM accounts WHERE user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Account, 0)
	for rows.Next() {
		var a Account
		var tls, active int
		var lastSynced *int64
		var createdAt, updatedAt int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Provider, &a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort, &a.Username, &tls, &a.OAuthClientID, &a.VaultCredentialID, &lastSynced, &a.SyncError, &active, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		a.UseTLS = tls == 1
		a.IsActive = active == 1
		a.LastSyncedAt = lastSynced
		a.CreatedAt = time.Unix(createdAt, 0)
		a.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &a)
	}
	return out, nil
}

func (s *Store) ListAllAccounts(ctx context.Context) ([]*Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, email, COALESCE(provider,'imap'), imap_host, imap_port, smtp_host, smtp_port, username, use_tls, COALESCE(oauth_client_id,''), COALESCE(vault_credential_id,''), last_synced_at, COALESCE(sync_error,''), is_active, created_at, updated_at FROM accounts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Account, 0)
	for rows.Next() {
		var a Account
		var tls, active int
		var lastSynced *int64
		var createdAt, updatedAt int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Provider, &a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort, &a.Username, &tls, &a.OAuthClientID, &a.VaultCredentialID, &lastSynced, &a.SyncError, &active, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		a.UseTLS = tls == 1
		a.IsActive = active == 1
		a.LastSyncedAt = lastSynced
		a.CreatedAt = time.Unix(createdAt, 0)
		a.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &a)
	}
	return out, nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (*Account, error) {
	var a Account
	var tls, active int
	var lastSynced *int64
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT id, name, email, COALESCE(provider,'imap'), imap_host, imap_port, smtp_host, smtp_port, username, password, COALESCE(oauth_client_id,''), COALESCE(oauth_client_secret,''), use_tls, last_synced_at, COALESCE(sync_error,''), is_active, created_at, updated_at FROM accounts WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.Email, &a.Provider, &a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort, &a.Username, &a.Password, &a.OAuthClientID, &a.OAuthClientSecret, &tls, &lastSynced, &a.SyncError, &active, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	a.UseTLS = tls == 1
	a.IsActive = active == 1
	a.LastSyncedAt = lastSynced
	a.CreatedAt = time.Unix(createdAt, 0)
	a.UpdatedAt = time.Unix(updatedAt, 0)
	a.Password, _ = client.Decrypt(a.Password)
	a.OAuthClientSecret, _ = client.Decrypt(a.OAuthClientSecret)
	return &a, nil
}

func (s *Store) UpdateAccount(ctx context.Context, id, name, email, provider, imapHost string, imapPort int, smtpHost string, smtpPort int, username, password, oauthClientID, oauthClientSecret string, useTLS bool) error {
	now := time.Now().UTC().Unix()
	tls := 0
	if useTLS {
		tls = 1
	}
	if provider == "" {
		provider = "imap"
	}
	encPass, _ := client.Encrypt(password)
	encSecret, _ := client.Encrypt(oauthClientSecret)
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET name=?, email=?, provider=?, imap_host=?, imap_port=?, smtp_host=?, smtp_port=?, username=?, password=?, oauth_client_id=?, oauth_client_secret=?, use_tls=?, updated_at=? WHERE id=?`,
		name, email, provider, imapHost, imapPort, smtpHost, smtpPort, username, encPass, oauthClientID, encSecret, tls, now, id)
	return err
}

func (s *Store) DeleteAccount(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", id)
	return err
}

func (s *Store) UpdateSyncStatus(ctx context.Context, id string, syncErr string) {
	now := time.Now().UTC().Unix()
	s.db.ExecContext(ctx, "UPDATE accounts SET last_synced_at = ?, sync_error = ?, updated_at = ? WHERE id = ?", now, syncErr, now, id)
}

// Folders

func (s *Store) UpsertFolder(ctx context.Context, accountID, name, remoteName string) (*Folder, error) {
	id := newID()
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO folders (id, account_id, name, remote_name, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(account_id, remote_name) DO UPDATE SET name = excluded.name`,
		id, accountID, name, remoteName, now)
	if err != nil {
		return nil, err
	}
	return s.GetFolderByRemoteName(ctx, accountID, remoteName)
}

func (s *Store) GetFolderByID(ctx context.Context, id string) (*Folder, error) {
	var f Folder
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, name, remote_name, unread_count, total_count, COALESCE(last_synced_uid,0) FROM folders WHERE id = ?`, id).
		Scan(&f.ID, &f.AccountID, &f.Name, &f.RemoteName, &f.UnreadCount, &f.TotalCount, &f.LastSyncedUID)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *Store) GetFolderByRemoteName(ctx context.Context, accountID, remoteName string) (*Folder, error) {
	var f Folder
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, name, remote_name, unread_count, total_count, COALESCE(last_synced_uid,0) FROM folders WHERE account_id = ? AND remote_name = ?`, accountID, remoteName).
		Scan(&f.ID, &f.AccountID, &f.Name, &f.RemoteName, &f.UnreadCount, &f.TotalCount, &f.LastSyncedUID)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *Store) ListFolders(ctx context.Context, accountID string) ([]*Folder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, name, remote_name, unread_count, total_count, COALESCE(last_synced_uid,0) FROM folders WHERE account_id = ? ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Folder, 0)
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.AccountID, &f.Name, &f.RemoteName, &f.UnreadCount, &f.TotalCount, &f.LastSyncedUID); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, nil
}

func (s *Store) UpdateFolderCounts(ctx context.Context, folderID string, total, unread int) {
	s.db.ExecContext(ctx, "UPDATE folders SET total_count = ?, unread_count = ? WHERE id = ?", total, unread, folderID)
}

// Emails

func (s *Store) InsertEmail(ctx context.Context, accountID, folderID, messageID, subject, fromName, fromAddr, toAddrs, snippet, bodyText, bodyHTML string, date int64, uid uint32, hasAttach, isRead bool, threadID, inReplyTo, unsubscribeURL string, unsubscribePost bool) error {
	id := newID()
	now := time.Now().UTC().Unix()
	attach := 0
	if hasAttach {
		attach = 1
	}
	read := 0
	if isRead {
		read = 1
	}
	unsubPost := 0
	if unsubscribePost {
		unsubPost = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO emails (id, account_id, folder_id, message_id, subject, from_name, from_address, to_addresses, date, snippet, body_text, body_html, is_read, is_starred, has_attachments, uid, fetched_at, thread_id, in_reply_to, unsubscribe_url, unsubscribe_post) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, folder_id, uid) DO UPDATE SET is_read=?, thread_id=?, in_reply_to=?`,
		id, accountID, folderID, messageID, subject, fromName, fromAddr, toAddrs, date, snippet, bodyText, bodyHTML, read, attach, uid, now, threadID, inReplyTo, unsubscribeURL, unsubPost,
		read, threadID, inReplyTo)
	return err
}

func (s *Store) ListEmails(ctx context.Context, folderID string, limit, offset int) ([]*Email, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.account_id, e.folder_id, e.message_id, e.subject, e.from_name, e.from_address, e.to_addresses, e.date, e.snippet, e.body_text, e.body_html, e.is_read, e.is_starred, e.has_attachments, e.is_dangerous, COALESCE(e.unsubscribe_url,''), e.unsubscribe_post,
			COALESCE(e.thread_id,''),
			CASE WHEN e.thread_id != '' THEN (SELECT COUNT(*) FROM emails e2 WHERE (e2.thread_id = e.thread_id OR e2.message_id = e.thread_id) AND e2.deleted_at IS NULL) ELSE 1 END
		FROM emails e
		WHERE e.folder_id = ? AND e.deleted_at IS NULL
		ORDER BY e.date DESC LIMIT ? OFFSET ?`, folderID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

func (s *Store) GetEmail(ctx context.Context, id string) (*Email, error) {
	var e Email
	var date int64
	var isRead, isStarred, hasAttach int
	var isDangerous, unsubPost int
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, folder_id, message_id, subject, from_name, from_address, to_addresses, date, snippet, body_text, body_html, is_read, is_starred, has_attachments, is_dangerous, COALESCE(unsubscribe_url,''), unsubscribe_post FROM emails WHERE id = ?`, id).
		Scan(&e.ID, &e.AccountID, &e.FolderID, &e.MessageID, &e.Subject, &e.FromName, &e.FromAddress, &e.ToAddresses, &date, &e.Snippet, &e.BodyText, &e.BodyHTML, &isRead, &isStarred, &hasAttach, &isDangerous, &e.UnsubscribeURL, &unsubPost)
	if err != nil {
		return nil, err
	}
	e.Date = time.Unix(date, 0)
	e.IsRead = isRead == 1
	e.IsStarred = isStarred == 1
	e.HasAttachments = hasAttach == 1
	e.IsDangerous = isDangerous == 1
	e.UnsubscribePost = unsubPost == 1
	return &e, nil
}

func (s *Store) MarkRead(ctx context.Context, id string) {
	s.db.ExecContext(ctx, "UPDATE emails SET is_read = 1 WHERE id = ?", id)
}

func (s *Store) MarkUnread(ctx context.Context, id string) {
	s.db.ExecContext(ctx, "UPDATE emails SET is_read = 0 WHERE id = ?", id)
}

func (s *Store) ToggleStar(ctx context.Context, id string) {
	s.db.ExecContext(ctx, "UPDATE emails SET is_starred = CASE WHEN is_starred = 1 THEN 0 ELSE 1 END WHERE id = ?", id)
}

func (s *Store) MarkDangerous(ctx context.Context, id string) {
	s.db.ExecContext(ctx, "UPDATE emails SET is_dangerous = 1 WHERE id = ?", id)
}

func (s *Store) SearchEmails(ctx context.Context, query string, limit int) ([]*Email, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.account_id, e.folder_id, e.message_id, e.subject, e.from_name, e.from_address, e.to_addresses, e.date, e.snippet, e.body_text, e.body_html, e.is_read, e.is_starred, e.has_attachments, e.is_dangerous, COALESCE(e.unsubscribe_url,''), e.unsubscribe_post FROM emails e JOIN emails_fts ON e.rowid = emails_fts.rowid WHERE emails_fts MATCH ? AND e.deleted_at IS NULL ORDER BY rank LIMIT ?`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

func (s *Store) SearchDeletedEmails(ctx context.Context, query string, limit int) ([]*Email, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.account_id, e.folder_id, e.message_id, e.subject, e.from_name, e.from_address, e.to_addresses, e.date, e.snippet, e.body_text, e.body_html, e.is_read, e.is_starred, e.has_attachments, e.is_dangerous, COALESCE(e.unsubscribe_url,''), e.unsubscribe_post FROM emails e JOIN emails_fts ON e.rowid = emails_fts.rowid WHERE emails_fts MATCH ? AND e.deleted_at IS NOT NULL ORDER BY rank LIMIT ?`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

func (s *Store) GetEmailUID(ctx context.Context, id string) (accountID, folderID string, uid uint32, err error) {
	var u int64
	err = s.db.QueryRowContext(ctx, `SELECT account_id, folder_id, uid FROM emails WHERE id = ?`, id).Scan(&accountID, &folderID, &u)
	uid = uint32(u)
	return
}

func (s *Store) GetFolderRemoteName(ctx context.Context, folderID string) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT remote_name FROM folders WHERE id = ?`, folderID).Scan(&name)
	return name, err
}

func (s *Store) SoftDeleteByMessageID(ctx context.Context, messageID string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE emails SET deleted_at = ? WHERE message_id = ? AND deleted_at IS NULL", now, messageID)
	return err
}

func (s *Store) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE emails SET deleted_at = ? WHERE id = ?", now, id)
	return err
}

func (s *Store) RestoreEmail(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE emails SET deleted_at = NULL WHERE id = ?", id)
	return err
}

func (s *Store) ListDeletedEmails(ctx context.Context, accountID string, limit, offset int) ([]*Email, error) {
	q := `SELECT id, account_id, folder_id, message_id, subject, from_name, from_address, to_addresses, date, snippet, body_text, body_html, is_read, is_starred, has_attachments, is_dangerous, COALESCE(unsubscribe_url,''), unsubscribe_post FROM emails WHERE deleted_at IS NOT NULL`
	args := make([]interface{}, 0)
	if accountID != "" {
		q += " AND account_id = ?"
		args = append(args, accountID)
	}
	q += " ORDER BY deleted_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

func scanEmails(rows *sql.Rows) ([]*Email, error) {
	out := make([]*Email, 0)
	cols, _ := rows.Columns()
	hasThread := len(cols) > 18
	for rows.Next() {
		var e Email
		var date int64
		var isRead, isStarred, hasAttach, isDangerous, unsubPost int
		if hasThread {
			if err := rows.Scan(&e.ID, &e.AccountID, &e.FolderID, &e.MessageID, &e.Subject, &e.FromName, &e.FromAddress, &e.ToAddresses, &date, &e.Snippet, &e.BodyText, &e.BodyHTML, &isRead, &isStarred, &hasAttach, &isDangerous, &e.UnsubscribeURL, &unsubPost, &e.ThreadID, &e.ThreadCount); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&e.ID, &e.AccountID, &e.FolderID, &e.MessageID, &e.Subject, &e.FromName, &e.FromAddress, &e.ToAddresses, &date, &e.Snippet, &e.BodyText, &e.BodyHTML, &isRead, &isStarred, &hasAttach, &isDangerous, &e.UnsubscribeURL, &unsubPost); err != nil {
				return nil, err
			}
		}
		e.Date = time.Unix(date, 0)
		e.IsRead = isRead == 1
		e.IsStarred = isStarred == 1
		e.HasAttachments = hasAttach == 1
		e.IsDangerous = isDangerous == 1
		e.UnsubscribePost = unsubPost == 1
		out = append(out, &e)
	}
	return out, nil
}

func (s *Store) ListThreadEmails(ctx context.Context, threadID string) ([]*Email, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, folder_id, message_id, subject, from_name, from_address, to_addresses, date, snippet, body_text, body_html, is_read, is_starred, has_attachments, is_dangerous, COALESCE(unsubscribe_url,''), unsubscribe_post FROM emails WHERE (thread_id = ? OR message_id = ?) AND deleted_at IS NULL ORDER BY date ASC`, threadID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

func (s *Store) UpdateLastSyncedUID(ctx context.Context, folderID string, uid uint32) {
	s.db.ExecContext(ctx, "UPDATE folders SET last_synced_uid = ? WHERE id = ? AND last_synced_uid < ?", uid, folderID, uid)
}

func (s *Store) ResetUnreadStatus(ctx context.Context, accountID string) {
	s.db.ExecContext(ctx, "UPDATE emails SET is_read = 1 WHERE account_id = ? AND deleted_at IS NULL", accountID)
}

func (s *Store) GetAccountUnreadCount(ctx context.Context, accountID string) int {
	var count int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM emails WHERE account_id = ? AND is_read = 0 AND deleted_at IS NULL", accountID).Scan(&count)
	return count
}

func (s *Store) SaveDraft(ctx context.Context, id, accountID, toAddresses, ccAddresses, bccAddresses, subject, bodyText, bodyHTML, inReplyTo string) (*Draft, error) {
	now := time.Now().UTC().Unix()
	if id == "" {
		id = newID()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO drafts (id, account_id, to_addresses, cc_addresses, bcc_addresses, subject, body_text, body_html, in_reply_to, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET account_id=?, to_addresses=?, cc_addresses=?, bcc_addresses=?, subject=?, body_text=?, body_html=?, in_reply_to=?, updated_at=?`,
		id, accountID, toAddresses, ccAddresses, bccAddresses, subject, bodyText, bodyHTML, inReplyTo, now,
		accountID, toAddresses, ccAddresses, bccAddresses, subject, bodyText, bodyHTML, inReplyTo, now)
	if err != nil {
		return nil, err
	}
	return &Draft{ID: id, AccountID: accountID, ToAddresses: toAddresses, CcAddresses: ccAddresses, BccAddresses: bccAddresses, Subject: subject, BodyText: bodyText, BodyHTML: bodyHTML, InReplyTo: inReplyTo, UpdatedAt: now}, nil
}

func (s *Store) GetDraft(ctx context.Context, id string) (*Draft, error) {
	var d Draft
	err := s.db.QueryRowContext(ctx, "SELECT id, account_id, to_addresses, cc_addresses, bcc_addresses, subject, body_text, body_html, in_reply_to, updated_at FROM drafts WHERE id = ?", id).
		Scan(&d.ID, &d.AccountID, &d.ToAddresses, &d.CcAddresses, &d.BccAddresses, &d.Subject, &d.BodyText, &d.BodyHTML, &d.InReplyTo, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListDrafts(ctx context.Context) ([]*Draft, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, account_id, to_addresses, cc_addresses, bcc_addresses, subject, body_text, body_html, in_reply_to, updated_at FROM drafts ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Draft, 0)
	for rows.Next() {
		var d Draft
		if err := rows.Scan(&d.ID, &d.AccountID, &d.ToAddresses, &d.CcAddresses, &d.BccAddresses, &d.Subject, &d.BodyText, &d.BodyHTML, &d.InReplyTo, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	return out, nil
}

func (s *Store) DeleteDraft(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ?", id)
	return err
}

func (s *Store) InsertAttachment(ctx context.Context, emailID, accountID, filename, contentType, storagePath string, sizeBytes int64) (*Attachment, error) {
	id := newID()
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "INSERT INTO attachments (id, email_id, account_id, filename, content_type, size_bytes, storage_path, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, emailID, accountID, filename, contentType, sizeBytes, storagePath, now)
	if err != nil {
		return nil, err
	}
	return &Attachment{ID: id, EmailID: emailID, AccountID: accountID, Filename: filename, ContentType: contentType, SizeBytes: sizeBytes, StoragePath: storagePath, CreatedAt: now}, nil
}

func (s *Store) ListAttachments(ctx context.Context, emailID string) ([]*Attachment, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, email_id, account_id, filename, content_type, size_bytes, storage_path, created_at FROM attachments WHERE email_id = ?", emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Attachment
	for rows.Next() {
		var a Attachment
		rows.Scan(&a.ID, &a.EmailID, &a.AccountID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.StoragePath, &a.CreatedAt)
		out = append(out, &a)
	}
	return out, nil
}

// Filters

func (s *Store) CreateFilter(ctx context.Context, accountID, rule string) (*Filter, error) {
	if _, err := ParseFilter(rule); err != nil {
		return nil, fmt.Errorf("invalid filter rule: %w", err)
	}
	id := newID()
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "INSERT INTO filters (id, account_id, rule, is_active, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
		id, accountID, rule, now, now)
	if err != nil {
		return nil, err
	}
	return &Filter{ID: id, AccountID: accountID, Rule: rule, IsActive: true, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) UpdateFilter(ctx context.Context, id, rule string, isActive bool) error {
	if _, err := ParseFilter(rule); err != nil {
		return fmt.Errorf("invalid filter rule: %w", err)
	}
	now := time.Now().UTC().Unix()
	active := 0
	if isActive {
		active = 1
	}
	_, err := s.db.ExecContext(ctx, "UPDATE filters SET rule = ?, is_active = ?, updated_at = ? WHERE id = ?", rule, active, now, id)
	return err
}

func (s *Store) DeleteFilter(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM filters WHERE id = ?", id)
	return err
}

func (s *Store) ListFilters(ctx context.Context, accountID string) ([]*Filter, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, account_id, rule, is_active, created_at, updated_at FROM filters WHERE account_id = ? ORDER BY created_at", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Filter
	for rows.Next() {
		var f Filter
		var active int
		if err := rows.Scan(&f.ID, &f.AccountID, &f.Rule, &active, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.IsActive = active == 1
		out = append(out, &f)
	}
	return out, nil
}

func (s *Store) ListActiveFilters(ctx context.Context, accountID string) ([]*ParsedFilter, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT rule FROM filters WHERE account_id = ? AND is_active = 1 ORDER BY created_at", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ParsedFilter
	for rows.Next() {
		var rule string
		if err := rows.Scan(&rule); err != nil {
			return nil, err
		}
		pf, err := ParseFilter(rule)
		if err != nil {
			continue
		}
		out = append(out, pf)
	}
	return out, nil
}

func (s *Store) GetFilter(ctx context.Context, id string) (*Filter, error) {
	var f Filter
	var active int
	err := s.db.QueryRowContext(ctx, "SELECT id, account_id, rule, is_active, created_at, updated_at FROM filters WHERE id = ?", id).
		Scan(&f.ID, &f.AccountID, &f.Rule, &active, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	f.IsActive = active == 1
	return &f, nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
