package email

import "time"

type Account struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Email             string    `json:"email"`
	Provider          string    `json:"provider"`
	IMAPHost          string    `json:"imap_host"`
	IMAPPort          int       `json:"imap_port"`
	SMTPHost          string    `json:"smtp_host"`
	SMTPPort          int       `json:"smtp_port"`
	Username          string    `json:"username"`
	Password          string    `json:"-"`
	OAuthClientID     string    `json:"oauth_client_id,omitempty"`
	OAuthClientSecret string    `json:"-"`
	AccessToken       string    `json:"-"`
	RefreshToken      string    `json:"-"`
	TokenExpiry       int64     `json:"token_expiry,omitempty"`
	VaultCredentialID string    `json:"vault_credential_id,omitempty"`
	UseTLS            bool      `json:"use_tls"`
	LastSyncedAt      *int64    `json:"last_synced_at,omitempty"`
	SyncError         string    `json:"sync_error,omitempty"`
	IsActive          bool      `json:"is_active"`
	UnreadCount       int       `json:"unread_count,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (a *Account) NeedsOAuth() bool {
	return a.Provider == "gmail-oauth"
}

func (a *Account) HasValidToken() bool {
	return a.AccessToken != "" && a.TokenExpiry > time.Now().UTC().Unix()
}

type Folder struct {
	ID            string `json:"id"`
	AccountID     string `json:"account_id"`
	Name          string `json:"name"`
	RemoteName    string `json:"remote_name"`
	UnreadCount   int    `json:"unread_count"`
	TotalCount    int    `json:"total_count"`
	LastSyncedUID uint32 `json:"last_synced_uid,omitempty"`
}

type Email struct {
	ID              string    `json:"id"`
	AccountID       string    `json:"account_id"`
	FolderID        string    `json:"folder_id"`
	MessageID       string    `json:"message_id"`
	ThreadID        string    `json:"thread_id,omitempty"`
	Subject         string    `json:"subject"`
	FromName        string    `json:"from_name"`
	FromAddress     string    `json:"from_address"`
	ToAddresses     string    `json:"to_addresses"`
	Date            time.Time `json:"date"`
	Snippet         string    `json:"snippet"`
	BodyText        string    `json:"body_text,omitempty"`
	BodyHTML        string    `json:"body_html,omitempty"`
	IsRead          bool      `json:"is_read"`
	IsStarred       bool      `json:"is_starred"`
	IsDangerous     bool      `json:"is_dangerous"`
	HasAttachments  bool      `json:"has_attachments"`
	UnsubscribeURL  string    `json:"unsubscribe_url,omitempty"`
	UnsubscribePost bool      `json:"unsubscribe_post,omitempty"`
	ThreadCount     int       `json:"thread_count,omitempty"`
}

type AttachmentFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

type Attachment struct {
	ID          string `json:"id"`
	EmailID     string `json:"email_id"`
	AccountID   string `json:"account_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	StoragePath string `json:"storage_path"`
	CreatedAt   int64  `json:"created_at"`
}

type Draft struct {
	ID           string `json:"id"`
	AccountID    string `json:"account_id"`
	ToAddresses  string `json:"to_addresses"`
	CcAddresses  string `json:"cc_addresses"`
	BccAddresses string `json:"bcc_addresses"`
	Subject      string `json:"subject"`
	BodyText     string `json:"body_text"`
	BodyHTML     string `json:"body_html"`
	InReplyTo    string `json:"in_reply_to"`
	UpdatedAt    int64  `json:"updated_at"`
}

type ComposeRequest struct {
	AccountID string   `json:"account_id"`
	To        []string `json:"to"`
	Cc        []string `json:"cc"`
	Bcc       []string `json:"bcc"`
	Subject   string   `json:"subject"`
	Body      string   `json:"body"`
	BodyHTML  string   `json:"body_html"`
	InReplyTo string   `json:"in_reply_to"`
}

type Filter struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Rule      string `json:"rule"`
	IsActive  bool   `json:"is_active"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type ConditionOp int

const (
	CondAnd ConditionOp = iota
	CondOr
	CondNot
	CondLeaf
)

type Condition struct {
	Op       ConditionOp
	Field    string
	Match    MatchType
	Value    string
	Children []*Condition
}

type MatchType int

const (
	MatchContains MatchType = iota
	MatchExact
	MatchStartsWith
	MatchEndsWith
)

type Action struct {
	Type  string
	Value string
}

type ParsedFilter struct {
	Condition *Condition `json:"condition"`
	Actions   []Action   `json:"actions"`
}

// Well-known IMAP/SMTP presets
type ProviderPreset struct {
	Name     string `json:"name"`
	IMAPHost string `json:"imap_host"`
	IMAPPort int    `json:"imap_port"`
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
}

var Presets = []ProviderPreset{
	{Name: "Gmail (OAuth)", IMAPHost: "imap.gmail.com", IMAPPort: 993, SMTPHost: "smtp.gmail.com", SMTPPort: 587},
	{Name: "Gmail (App Password)", IMAPHost: "imap.gmail.com", IMAPPort: 993, SMTPHost: "smtp.gmail.com", SMTPPort: 587},
	{Name: "Yahoo", IMAPHost: "imap.mail.yahoo.com", IMAPPort: 993, SMTPHost: "smtp.mail.yahoo.com", SMTPPort: 587},
	{Name: "Outlook", IMAPHost: "outlook.office365.com", IMAPPort: 993, SMTPHost: "smtp.office365.com", SMTPPort: 587},
	{Name: "iCloud", IMAPHost: "imap.mail.me.com", IMAPPort: 993, SMTPHost: "smtp.mail.me.com", SMTPPort: 587},
	{Name: "Fastmail", IMAPHost: "imap.fastmail.com", IMAPPort: 993, SMTPHost: "smtp.fastmail.com", SMTPPort: 587},
	{Name: "ProtonMail Bridge", IMAPHost: "127.0.0.1", IMAPPort: 1143, SMTPHost: "127.0.0.1", SMTPPort: 1025},
}
