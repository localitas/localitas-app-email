package email

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/localitas/localitas-go"
)

const DatabaseName = "email"

// ErrNotFound is returned for a record that does not exist or belongs to
// another user: the two are indistinguishable to the caller.
var ErrNotFound = errors.New("not found")

// ErrInvalidFilter is returned for a filter rule that does not parse.
var ErrInvalidFilter = errors.New("invalid filter rule")

// ownedAccounts selects the IDs of the accounts a user owns; every record
// other than an account belongs to a user through its account_id.
const ownedAccounts = "SELECT id FROM accounts WHERE user_id = ?"

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
	encPass, err := client.Encrypt(password)
	if err != nil {
		return nil, fmt.Errorf("encrypt password: %w", err)
	}
	encSecret, err := client.Encrypt(oauthClientSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt oauth_client_secret: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO accounts (id, user_id, name, email, provider, imap_host, imap_port, smtp_host, smtp_port, username, password, oauth_client_id, oauth_client_secret, use_tls, vault_credential_id, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, userID, name, email, provider, imapHost, imapPort, smtpHost, smtpPort, username, encPass, oauthClientID, encSecret, tls, vaultCredentialID, now, now)
	if err != nil {
		return nil, err
	}
	return &Account{ID: id, UserID: userID, Name: name, Email: email, Provider: provider, IMAPHost: imapHost, IMAPPort: imapPort, SMTPHost: smtpHost, SMTPPort: smtpPort, Username: username, OAuthClientID: oauthClientID, VaultCredentialID: vaultCredentialID, UseTLS: useTLS, IsActive: true, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

const accountListColumns = `id, user_id, name, email, COALESCE(provider,'imap'), imap_host, imap_port, smtp_host, smtp_port, username, use_tls, COALESCE(oauth_client_id,''), COALESCE(vault_credential_id,''), last_synced_at, COALESCE(sync_error,''), is_active, created_at, updated_at`

func scanAccounts(rows *sql.Rows) ([]*Account, error) {
	out := make([]*Account, 0)
	for rows.Next() {
		var a Account
		var tls, active int
		var lastSynced *int64
		var createdAt, updatedAt int64
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Email, &a.Provider, &a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort, &a.Username, &tls, &a.OAuthClientID, &a.VaultCredentialID, &lastSynced, &a.SyncError, &active, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		a.UseTLS = tls == 1
		a.IsActive = active == 1
		a.LastSyncedAt = lastSynced
		a.CreatedAt = time.Unix(createdAt, 0)
		a.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ListAccounts returns userID's accounts, without their secrets.
func (s *Store) ListAccounts(ctx context.Context, userID string) ([]*Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountListColumns+` FROM accounts WHERE user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

// ListAllAccounts returns every user's accounts, without their secrets. Only
// the system sync job may use it.
func (s *Store) ListAllAccounts(ctx context.Context) ([]*Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountListColumns+` FROM accounts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

// RequireAccount returns ErrNotFound unless userID owns account id.
func (s *Store) RequireAccount(ctx context.Context, userID, id string) error {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id = ? AND user_id = ?`, id, userID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read account %s: %w", id, err)
	}
	return nil
}

// GetAccount returns userID's account id with its decrypted password and
// OAuth client secret.
func (s *Store) GetAccount(ctx context.Context, userID, id string) (*Account, error) {
	var a Account
	var tls, active int
	var lastSynced *int64
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, name, email, COALESCE(provider,'imap'), imap_host, imap_port, smtp_host, smtp_port, username, password, COALESCE(oauth_client_id,''), COALESCE(oauth_client_secret,''), use_tls, last_synced_at, COALESCE(sync_error,''), is_active, created_at, updated_at FROM accounts WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&a.ID, &a.UserID, &a.Name, &a.Email, &a.Provider, &a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort, &a.Username, &a.Password, &a.OAuthClientID, &a.OAuthClientSecret, &tls, &lastSynced, &a.SyncError, &active, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read account %s: %w", id, err)
	}
	a.UseTLS = tls == 1
	a.IsActive = active == 1
	a.LastSyncedAt = lastSynced
	a.CreatedAt = time.Unix(createdAt, 0)
	a.UpdatedAt = time.Unix(updatedAt, 0)
	if a.Password, err = client.Decrypt(a.Password); err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}
	if a.OAuthClientSecret, err = client.Decrypt(a.OAuthClientSecret); err != nil {
		return nil, fmt.Errorf("decrypt oauth_client_secret: %w", err)
	}
	return &a, nil
}

// UpdateAccount rewrites userID's account id. An update that omits a secret
// keeps the stored one.
func (s *Store) UpdateAccount(ctx context.Context, userID, id, name, email, provider, imapHost string, imapPort int, smtpHost string, smtpPort int, username, password, oauthClientID, oauthClientSecret string, useTLS bool) error {
	now := time.Now().UTC().Unix()
	tls := 0
	if useTLS {
		tls = 1
	}
	if provider == "" {
		provider = "imap"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET name=?, email=?, provider=?, imap_host=?, imap_port=?, smtp_host=?, smtp_port=?, username=?, oauth_client_id=?, use_tls=?, updated_at=? WHERE id=? AND user_id=?`,
		name, email, provider, imapHost, imapPort, smtpHost, smtpPort, username, oauthClientID, tls, now, id, userID)
	if err := requireOneRow(res, err); err != nil {
		return err
	}
	// Writing Encrypt("") would silently blank the stored secret.
	if password != "" {
		encPass, err := client.Encrypt(password)
		if err != nil {
			return fmt.Errorf("encrypt password: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE accounts SET password=? WHERE id=? AND user_id=?`, encPass, id, userID); err != nil {
			return err
		}
	}
	if oauthClientSecret != "" {
		encSecret, err := client.Encrypt(oauthClientSecret)
		if err != nil {
			return fmt.Errorf("encrypt oauth_client_secret: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE accounts SET oauth_client_secret=? WHERE id=? AND user_id=?`, encSecret, id, userID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteAccount removes userID's account id and everything that belongs to
// it. The schema's ON DELETE CASCADE clauses are not relied on: SQLite only
// enforces foreign keys on connections with PRAGMA foreign_keys=ON, so without
// explicit deletes the account's mail would be orphaned and keep appearing in
// search. Deleting emails fires the emails_ad trigger, keeping the FTS index
// in sync. Each delete is scoped to the owner, so another user's account is
// left untouched and reported as ErrNotFound.
func (s *Store) DeleteAccount(ctx context.Context, userID, id string) error {
	for _, table := range []string{"attachments", "drafts", "filters", "emails", "folders"} {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE account_id IN (SELECT id FROM accounts WHERE id = ? AND user_id = ?)", id, userID); err != nil {
			return fmt.Errorf("delete %s for account %s: %w", table, id, err)
		}
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ? AND user_id = ?", id, userID)
	return requireOneRow(res, err)
}

func (s *Store) UpdateSyncStatus(ctx context.Context, id string, syncErr string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE accounts SET last_synced_at = ?, sync_error = ?, updated_at = ? WHERE id = ?", now, syncErr, now, id)
	return err
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

// GetFolderByID returns userID's folder id.
func (s *Store) GetFolderByID(ctx context.Context, userID, id string) (*Folder, error) {
	var f Folder
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, name, remote_name, unread_count, total_count, COALESCE(last_synced_uid,0) FROM folders WHERE id = ? AND account_id IN (`+ownedAccounts+`)`, id, userID).
		Scan(&f.ID, &f.AccountID, &f.Name, &f.RemoteName, &f.UnreadCount, &f.TotalCount, &f.LastSyncedUID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read folder %s: %w", id, err)
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

// ListFolders returns the folders of userID's account accountID.
func (s *Store) ListFolders(ctx context.Context, userID, accountID string) ([]*Folder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, name, remote_name, unread_count, total_count, COALESCE(last_synced_uid,0) FROM folders WHERE account_id = ? AND account_id IN (`+ownedAccounts+`) ORDER BY name`, accountID, userID)
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
	return out, rows.Err()
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

const emailColumns = `e.id, e.account_id, e.folder_id, e.message_id, e.subject, e.from_name, e.from_address, e.to_addresses, e.date, e.snippet, e.body_text, e.body_html, e.is_read, e.is_starred, e.has_attachments, e.is_dangerous, COALESCE(e.unsubscribe_url,''), e.unsubscribe_post`

// ListEmails returns the live emails in userID's folder folderID.
func (s *Store) ListEmails(ctx context.Context, userID, folderID string, limit, offset int) ([]*Email, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+emailColumns+`,
			COALESCE(e.thread_id,''),
			CASE WHEN e.thread_id != '' THEN (SELECT COUNT(*) FROM emails e2 WHERE e2.account_id = e.account_id AND (e2.thread_id = e.thread_id OR e2.message_id = e.thread_id) AND e2.deleted_at IS NULL) ELSE 1 END
		FROM emails e
		WHERE e.folder_id = ? AND e.account_id IN (`+ownedAccounts+`) AND e.deleted_at IS NULL
		ORDER BY e.date DESC LIMIT ? OFFSET ?`, folderID, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

// GetEmail returns userID's email id, live or in the trash.
func (s *Store) GetEmail(ctx context.Context, userID, id string) (*Email, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+emailColumns+` FROM emails e WHERE e.id = ? AND e.account_id IN (`+ownedAccounts+`)`, id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails, err := scanEmails(rows)
	if err != nil {
		return nil, fmt.Errorf("read email %s: %w", id, err)
	}
	if len(emails) == 0 {
		return nil, ErrNotFound
	}
	return emails[0], nil
}

// updateOwnedEmail runs set on userID's email id.
func (s *Store) updateOwnedEmail(ctx context.Context, userID, id, set string, args ...any) error {
	args = append(args, id, userID)
	res, err := s.db.ExecContext(ctx, "UPDATE emails SET "+set+" WHERE id = ? AND account_id IN ("+ownedAccounts+")", args...)
	return requireOneRow(res, err)
}

func (s *Store) MarkRead(ctx context.Context, userID, id string) error {
	return s.updateOwnedEmail(ctx, userID, id, "is_read = 1")
}

func (s *Store) MarkUnread(ctx context.Context, userID, id string) error {
	return s.updateOwnedEmail(ctx, userID, id, "is_read = 0")
}

func (s *Store) ToggleStar(ctx context.Context, userID, id string) error {
	return s.updateOwnedEmail(ctx, userID, id, "is_starred = CASE WHEN is_starred = 1 THEN 0 ELSE 1 END")
}

func (s *Store) SoftDelete(ctx context.Context, userID, id string) error {
	return s.updateOwnedEmail(ctx, userID, id, "deleted_at = ?", time.Now().UTC().Unix())
}

func (s *Store) RestoreEmail(ctx context.Context, userID, id string) error {
	return s.updateOwnedEmail(ctx, userID, id, "deleted_at = NULL")
}

func (s *Store) MarkDangerous(ctx context.Context, id string) {
	s.db.ExecContext(ctx, "UPDATE emails SET is_dangerous = 1 WHERE id = ?", id)
}

// SearchEmails full-text searches userID's live emails.
func (s *Store) SearchEmails(ctx context.Context, userID, query string, limit int) ([]*Email, error) {
	return s.search(ctx, userID, query, limit, "e.deleted_at IS NULL")
}

// SearchDeletedEmails full-text searches userID's trashed emails.
func (s *Store) SearchDeletedEmails(ctx context.Context, userID, query string, limit int) ([]*Email, error) {
	return s.search(ctx, userID, query, limit, "e.deleted_at IS NOT NULL")
}

func (s *Store) search(ctx context.Context, userID, query string, limit int, deleted string) ([]*Email, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+emailColumns+` FROM emails e JOIN emails_fts ON e.rowid = emails_fts.rowid WHERE emails_fts MATCH ? AND e.account_id IN (`+ownedAccounts+`) AND `+deleted+` ORDER BY rank LIMIT ?`, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

// GetEmailUID returns where userID's email id lives on the IMAP server.
func (s *Store) GetEmailUID(ctx context.Context, userID, id string) (accountID, folderID string, uid uint32, err error) {
	var u int64
	err = s.db.QueryRowContext(ctx, `SELECT account_id, folder_id, uid FROM emails WHERE id = ? AND account_id IN (`+ownedAccounts+`)`, id, userID).Scan(&accountID, &folderID, &u)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, ErrNotFound
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("read email %s: %w", id, err)
	}
	return accountID, folderID, uint32(u), nil
}

func (s *Store) GetFolderRemoteName(ctx context.Context, accountID, folderID string) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT remote_name FROM folders WHERE id = ? AND account_id = ?`, folderID, accountID).Scan(&name)
	return name, err
}

// SoftDeleteByMessageID trashes the copies of messageID in accountID; a
// message in one account's trash says nothing about another account's mail.
func (s *Store) SoftDeleteByMessageID(ctx context.Context, accountID, messageID string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE emails SET deleted_at = ? WHERE account_id = ? AND message_id = ? AND deleted_at IS NULL", now, accountID, messageID)
	return err
}

// ListDeletedEmails returns userID's trashed emails, optionally only those of
// accountID.
func (s *Store) ListDeletedEmails(ctx context.Context, userID, accountID string, limit, offset int) ([]*Email, error) {
	q := `SELECT ` + emailColumns + ` FROM emails e WHERE e.deleted_at IS NOT NULL AND e.account_id IN (` + ownedAccounts + `)`
	args := []any{userID}
	if accountID != "" {
		q += " AND e.account_id = ?"
		args = append(args, accountID)
	}
	q += " ORDER BY e.deleted_at DESC LIMIT ? OFFSET ?"
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
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	hasThread := len(cols) > 18
	for rows.Next() {
		var e Email
		var date int64
		var isRead, isStarred, hasAttach, isDangerous, unsubPost int
		dest := []any{&e.ID, &e.AccountID, &e.FolderID, &e.MessageID, &e.Subject, &e.FromName, &e.FromAddress, &e.ToAddresses, &date, &e.Snippet, &e.BodyText, &e.BodyHTML, &isRead, &isStarred, &hasAttach, &isDangerous, &e.UnsubscribeURL, &unsubPost}
		if hasThread {
			dest = append(dest, &e.ThreadID, &e.ThreadCount)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		e.Date = time.Unix(date, 0)
		e.IsRead = isRead == 1
		e.IsStarred = isStarred == 1
		e.HasAttachments = hasAttach == 1
		e.IsDangerous = isDangerous == 1
		e.UnsubscribePost = unsubPost == 1
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ListThreadEmails returns the live emails of thread threadID in userID's
// accounts.
func (s *Store) ListThreadEmails(ctx context.Context, userID, threadID string) ([]*Email, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+emailColumns+` FROM emails e WHERE (e.thread_id = ? OR e.message_id = ?) AND e.account_id IN (`+ownedAccounts+`) AND e.deleted_at IS NULL ORDER BY e.date ASC`, threadID, threadID, userID)
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

func (s *Store) GetAccountUnreadCount(ctx context.Context, accountID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM emails WHERE account_id = ? AND is_read = 0 AND deleted_at IS NULL", accountID).Scan(&count)
	return count, err
}

// Drafts

const draftColumns = "id, account_id, to_addresses, cc_addresses, bcc_addresses, subject, body_text, body_html, in_reply_to, updated_at"

// SaveDraft creates a draft in userID's account accountID when id is empty,
// and otherwise rewrites userID's draft id. Another user's draft is
// ErrNotFound and is left untouched.
func (s *Store) SaveDraft(ctx context.Context, userID, id, accountID, toAddresses, ccAddresses, bccAddresses, subject, bodyText, bodyHTML, inReplyTo string) (*Draft, error) {
	if err := s.RequireAccount(ctx, userID, accountID); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Unix()
	if id == "" {
		id = newID()
		if _, err := s.db.ExecContext(ctx, `INSERT INTO drafts (`+draftColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, accountID, toAddresses, ccAddresses, bccAddresses, subject, bodyText, bodyHTML, inReplyTo, now); err != nil {
			return nil, err
		}
	} else {
		res, err := s.db.ExecContext(ctx, `UPDATE drafts SET account_id=?, to_addresses=?, cc_addresses=?, bcc_addresses=?, subject=?, body_text=?, body_html=?, in_reply_to=?, updated_at=? WHERE id = ? AND account_id IN (`+ownedAccounts+`)`,
			accountID, toAddresses, ccAddresses, bccAddresses, subject, bodyText, bodyHTML, inReplyTo, now, id, userID)
		if err := requireOneRow(res, err); err != nil {
			return nil, err
		}
	}
	return &Draft{ID: id, AccountID: accountID, ToAddresses: toAddresses, CcAddresses: ccAddresses, BccAddresses: bccAddresses, Subject: subject, BodyText: bodyText, BodyHTML: bodyHTML, InReplyTo: inReplyTo, UpdatedAt: now}, nil
}

func scanDrafts(rows *sql.Rows) ([]*Draft, error) {
	out := make([]*Draft, 0)
	for rows.Next() {
		var d Draft
		if err := rows.Scan(&d.ID, &d.AccountID, &d.ToAddresses, &d.CcAddresses, &d.BccAddresses, &d.Subject, &d.BodyText, &d.BodyHTML, &d.InReplyTo, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

// GetDraft returns userID's draft id.
func (s *Store) GetDraft(ctx context.Context, userID, id string) (*Draft, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+draftColumns+" FROM drafts WHERE id = ? AND account_id IN ("+ownedAccounts+")", id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	drafts, err := scanDrafts(rows)
	if err != nil {
		return nil, fmt.Errorf("read draft %s: %w", id, err)
	}
	if len(drafts) == 0 {
		return nil, ErrNotFound
	}
	return drafts[0], nil
}

// ListDrafts returns userID's drafts across their accounts.
func (s *Store) ListDrafts(ctx context.Context, userID string) ([]*Draft, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+draftColumns+" FROM drafts WHERE account_id IN ("+ownedAccounts+") ORDER BY updated_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDrafts(rows)
}

// DeleteDraft removes userID's draft id.
func (s *Store) DeleteDraft(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ? AND account_id IN ("+ownedAccounts+")", id, userID)
	return requireOneRow(res, err)
}

// Attachments

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

const attachmentColumns = "id, email_id, account_id, filename, content_type, size_bytes, storage_path, created_at"

func scanAttachments(rows *sql.Rows) ([]*Attachment, error) {
	out := make([]*Attachment, 0)
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.EmailID, &a.AccountID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.StoragePath, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ListAttachments returns the attachments of userID's email emailID.
func (s *Store) ListAttachments(ctx context.Context, userID, emailID string) ([]*Attachment, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+attachmentColumns+" FROM attachments WHERE email_id = ? AND account_id IN ("+ownedAccounts+")", emailID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttachments(rows)
}

// GetAttachment returns userID's attachment id.
func (s *Store) GetAttachment(ctx context.Context, userID, id string) (*Attachment, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+attachmentColumns+" FROM attachments WHERE id = ? AND account_id IN ("+ownedAccounts+")", id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	atts, err := scanAttachments(rows)
	if err != nil {
		return nil, fmt.Errorf("read attachment %s: %w", id, err)
	}
	if len(atts) == 0 {
		return nil, ErrNotFound
	}
	return atts[0], nil
}

// Filters

func validateFilter(rule string) error {
	if _, err := ParseFilter(rule); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilter, err)
	}
	return nil
}

// CreateFilter adds a filter to userID's account accountID.
func (s *Store) CreateFilter(ctx context.Context, userID, accountID, rule string) (*Filter, error) {
	if err := validateFilter(rule); err != nil {
		return nil, err
	}
	if err := s.RequireAccount(ctx, userID, accountID); err != nil {
		return nil, err
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

// UpdateFilter rewrites userID's filter id.
func (s *Store) UpdateFilter(ctx context.Context, userID, id, rule string, isActive bool) error {
	if err := validateFilter(rule); err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	active := 0
	if isActive {
		active = 1
	}
	res, err := s.db.ExecContext(ctx, "UPDATE filters SET rule = ?, is_active = ?, updated_at = ? WHERE id = ? AND account_id IN ("+ownedAccounts+")", rule, active, now, id, userID)
	return requireOneRow(res, err)
}

// DeleteFilter removes userID's filter id.
func (s *Store) DeleteFilter(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM filters WHERE id = ? AND account_id IN ("+ownedAccounts+")", id, userID)
	return requireOneRow(res, err)
}

func scanFilters(rows *sql.Rows) ([]*Filter, error) {
	out := make([]*Filter, 0)
	for rows.Next() {
		var f Filter
		var active int
		if err := rows.Scan(&f.ID, &f.AccountID, &f.Rule, &active, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.IsActive = active == 1
		out = append(out, &f)
	}
	return out, rows.Err()
}

// ListFilters returns the filters of userID's account accountID.
func (s *Store) ListFilters(ctx context.Context, userID, accountID string) ([]*Filter, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, account_id, rule, is_active, created_at, updated_at FROM filters WHERE account_id = ? AND account_id IN ("+ownedAccounts+") ORDER BY created_at", accountID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFilters(rows)
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
	return out, rows.Err()
}

func requireOneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
