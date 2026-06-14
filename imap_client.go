package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

type xoauth2Client struct {
	email string
	token string
}

func (c *xoauth2Client) Start() (string, []byte, error) {
	ir := []byte("user=" + c.email + "\x01auth=Bearer " + c.token + "\x01\x01")
	return "XOAUTH2", ir, nil
}

func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	return nil, fmt.Errorf("xoauth2: unexpected challenge")
}

type attachmentData struct {
	Filename    string
	ContentType string
	Data        []byte
}

type SyncConfig struct {
	CoreURL   string
	AuthToken string
}

type SyncResult struct {
	Folders    int      `json:"folders"`
	NewEmails  int      `json:"new_emails"`
	Trashed    int      `json:"trashed"`
	DurationMs int64    `json:"duration_ms"`
	Errors     []string `json:"errors,omitempty"`
}

func SyncSingleFolder(ctx context.Context, store *Store, account *Account, folder *Folder, maxMessages uint32, syncCfg *SyncConfig) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	if account.NeedsOAuth() {
		if err := EnsureValidToken(ctx, store, account); err != nil {
			return nil, fmt.Errorf("oauth: %w", err)
		}
	}

	c, err := connectIMAP(account)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	n, err := syncFolder(ctx, c, store, account.ID, folder, maxMessages, syncCfg)
	if err != nil {
		result.Errors = append(result.Errors, folder.Name+": "+err.Error())
	} else {
		result.NewEmails = n
	}
	result.Folders = 1
	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

func SyncAccount(ctx context.Context, store *Store, account *Account, maxMessages uint32, syncCfg *SyncConfig) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	if account.NeedsOAuth() {
		if err := EnsureValidToken(ctx, store, account); err != nil {
			return nil, fmt.Errorf("oauth: %w", err)
		}
	}

	c, err := connectIMAP(account)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	mailboxes := make(chan *imap.MailboxInfo, 50)
	go func() { c.List("", "*", mailboxes) }()

	var mbList []*imap.MailboxInfo
	for mb := range mailboxes {
		mbList = append(mbList, mb)
	}

	type folderToSync struct {
		folder *Folder
		name   string
	}
	var foldersToSync []folderToSync

	for _, mb := range mbList {
		folder, err := store.UpsertFolder(ctx, account.ID, mb.Name, mb.Name)
		if err != nil {
			log.Printf("⚠️  Failed to upsert folder %s: %v", mb.Name, err)
			continue
		}
		result.Folders++
		if shouldSyncFolder(mb.Name) {
			foldersToSync = append(foldersToSync, folderToSync{folder: folder, name: mb.Name})
		}
	}

	for _, fs := range foldersToSync {
		folderMax := maxMessages
		if strings.ToLower(fs.name) != "inbox" && fs.folder.LastSyncedUID == 0 {
			folderMax = 50
		}
		n, err := syncFolder(ctx, c, store, account.ID, fs.folder, folderMax, syncCfg)
		if err != nil {
			log.Printf("⚠️  Failed to sync folder %s: %v", fs.name, err)
			result.Errors = append(result.Errors, fs.name+": "+err.Error())
			continue
		}
		result.NewEmails += n
	}

	trashed, err := SyncTrashFolder(ctx, store, account, maxMessages)
	if err != nil {
		log.Printf("⚠️  Failed to sync trash: %v", err)
	}
	result.Trashed = trashed

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

func connectIMAP(account *Account) (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", account.IMAPHost, account.IMAPPort)
	var c *imapclient.Client
	var err error

	if account.UseTLS {
		c, err = imapclient.DialTLS(addr, nil)
	} else {
		c, err = imapclient.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	c.Timeout = 30 * time.Second

	if account.NeedsOAuth() && account.AccessToken != "" {
		saslClient := &xoauth2Client{email: account.Email, token: account.AccessToken}
		if err := c.Authenticate(saslClient); err != nil {
			c.Logout()
			return nil, fmt.Errorf("xoauth2 auth: %w", err)
		}
	} else {
		if err := c.Login(account.Username, account.Password); err != nil {
			c.Logout()
			return nil, fmt.Errorf("login: %w", err)
		}
	}

	return c, nil
}

func MoveToTrash(account *Account, sourceFolder string, uid uint32) error {
	c, err := connectIMAP(account)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	trashName := findTrashFolder(c)
	if trashName == "" {
		return fmt.Errorf("no trash folder found on server")
	}

	if _, err := c.Select(sourceFolder, false); err != nil {
		return fmt.Errorf("select %s: %w", sourceFolder, err)
	}

	uidSet := new(imap.SeqSet)
	uidSet.AddNum(uid)

	if err := c.UidCopy(uidSet, trashName); err != nil {
		return fmt.Errorf("copy to trash: %w", err)
	}

	flagItem := imap.FormatFlagsOp(imap.AddFlags, true)
	if err := c.UidStore(uidSet, flagItem, []interface{}{imap.DeletedFlag}, nil); err != nil {
		return fmt.Errorf("flag deleted: %w", err)
	}

	if err := c.Expunge(nil); err != nil {
		return fmt.Errorf("expunge: %w", err)
	}

	return nil
}

func findTrashFolder(c *imapclient.Client) string {
	mailboxes := make(chan *imap.MailboxInfo, 50)
	go func() { c.List("", "*", mailboxes) }()

	var trashName string
	for mb := range mailboxes {
		lower := strings.ToLower(mb.Name)
		if trashName == "" && (lower == "trash" || lower == "[gmail]/trash" || lower == "[gmail]/bin") {
			trashName = mb.Name
		}
	}
	if trashName == "" {
		return "Trash"
	}
	return trashName
}

func SyncTrashFolder(ctx context.Context, store *Store, account *Account, maxMessages uint32) (int, error) {
	c, err := connectIMAP(account)
	if err != nil {
		return 0, fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	trashName := findTrashFolder(c)

	mbox, err := c.Select(trashName, true)
	if err != nil {
		return 0, nil
	}
	if mbox.Messages == 0 {
		return 0, nil
	}

	from := uint32(1)
	if mbox.Messages > maxMessages {
		from = mbox.Messages - maxMessages + 1
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(from, mbox.Messages)

	messages := make(chan *imap.Message, 10)
	go func() { c.Fetch(seqSet, []imap.FetchItem{imap.FetchEnvelope}, messages) }()

	marked := 0
	for msg := range messages {
		if msg.Envelope == nil || msg.Envelope.MessageId == "" {
			continue
		}
		if err := store.SoftDeleteByMessageID(ctx, msg.Envelope.MessageId); err == nil {
			marked++
		}
	}

	return marked, nil
}

func SendEmail(account *Account, to, cc, bcc []string, subject, body, bodyHTML, inReplyTo string) error {
	from := account.Email
	addr := fmt.Sprintf("%s:%d", account.SMTPHost, account.SMTPPort)

	msgID := fmt.Sprintf("<%d.%s@localitas>", time.Now().UTC().UnixNano(), account.Email)
	headers := "From: " + from + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n"
	if len(cc) > 0 {
		headers += "Cc: " + strings.Join(cc, ", ") + "\r\n"
	}
	headers += "Message-ID: " + msgID + "\r\n"
	if inReplyTo != "" {
		headers += "In-Reply-To: " + inReplyTo + "\r\n" +
			"References: " + inReplyTo + "\r\n"
	}
	headers += "Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n"

	var msg string
	if bodyHTML != "" {
		boundary := "localitas-boundary-" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
		msg = headers +
			"Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n" +
			"\r\n" +
			"--" + boundary + "\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
			body + "\r\n" +
			"--" + boundary + "\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n\r\n" +
			bodyHTML + "\r\n" +
			"--" + boundary + "--\r\n"
	} else {
		msg = headers +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" + body
	}

	allRecipients := append(append(to, cc...), bcc...)
	var auth smtp.Auth
	if account.NeedsOAuth() && account.AccessToken != "" {
		auth = &xoauth2SMTPAuth{email: account.Email, token: account.AccessToken}
	} else {
		auth = smtp.PlainAuth("", account.Username, account.Password, account.SMTPHost)
	}
	return smtp.SendMail(addr, auth, from, allRecipients, []byte(msg))
}

type xoauth2SMTPAuth struct {
	email string
	token string
}

func (a *xoauth2SMTPAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	ir := []byte("user=" + a.email + "\x01auth=Bearer " + a.token + "\x01\x01")
	return "XOAUTH2", ir, nil
}

func (a *xoauth2SMTPAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return nil, fmt.Errorf("xoauth2 smtp: unexpected challenge")
	}
	return nil, nil
}

func resolveThreadID(ctx context.Context, store *Store, accountID, inReplyTo string) string {
	current := inReplyTo
	for i := 0; i < 10; i++ {
		var parentInReplyTo, parentThreadID string
		err := store.db.QueryRowContext(ctx,
			"SELECT COALESCE(in_reply_to,''), COALESCE(thread_id,'') FROM emails WHERE message_id = ? AND account_id = ?",
			current, accountID).Scan(&parentInReplyTo, &parentThreadID)
		if err != nil {
			return current
		}
		if parentThreadID != "" {
			return parentThreadID
		}
		if parentInReplyTo == "" {
			return current
		}
		current = parentInReplyTo
	}
	return current
}

func extractHeaderValue(headers, name string) string {
	lower := strings.ToLower(name) + ":"
	var result string
	capturing := false
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimRight(line, "\r")
		if capturing {
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				result += " " + strings.TrimSpace(line)
				continue
			}
			break
		}
		if strings.HasPrefix(strings.ToLower(line), lower) {
			result = strings.TrimSpace(line[len(lower):])
			capturing = true
		}
	}
	return result
}

func findUnsubscribeLinkInHTML(html string) string {
	lower := strings.ToLower(html)
	searchStart := 0
	for {
		idx := strings.Index(lower[searchStart:], "<a ")
		if idx < 0 {
			break
		}
		idx += searchStart
		endTag := strings.Index(lower[idx:], ">")
		if endTag < 0 {
			break
		}
		tagEnd := idx + endTag
		closeTag := strings.Index(lower[tagEnd:], "</a>")
		linkText := ""
		if closeTag > 0 {
			linkText = strings.ToLower(html[tagEnd+1 : tagEnd+closeTag])
		}

		tag := lower[idx : tagEnd+1]
		hrefIdx := strings.Index(tag, "href=\"")
		if hrefIdx < 0 {
			hrefIdx = strings.Index(tag, "href='")
		}
		if hrefIdx >= 0 {
			quote := tag[hrefIdx+5]
			urlStart := idx + hrefIdx + 6
			urlEnd := strings.IndexByte(lower[urlStart:], quote)
			if urlEnd > 0 {
				href := html[urlStart : urlStart+urlEnd]
				hrefLower := strings.ToLower(href)
				if strings.Contains(hrefLower, "unsubscribe") || strings.Contains(linkText, "unsubscribe") {
					if strings.HasPrefix(hrefLower, "http://") || strings.HasPrefix(hrefLower, "https://") {
						return href
					}
				}
			}
		}
		searchStart = tagEnd + 1
	}
	return ""
}

func parseUnsubscribeURL(header string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			url := part[1 : len(part)-1]
			if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
				return url
			}
			if strings.HasPrefix(url, "mailto:") {
				return url
			}
		}
	}
	return ""
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func SendEmailWithAttachments(account *Account, to, cc, bcc []string, subject, body, bodyHTML, inReplyTo string, attachments []AttachmentFile) error {
	if len(attachments) == 0 {
		return SendEmail(account, to, cc, bcc, subject, body, bodyHTML, inReplyTo)
	}

	from := account.Email
	addr := fmt.Sprintf("%s:%d", account.SMTPHost, account.SMTPPort)

	boundary := fmt.Sprintf("localitas-mixed-%d", time.Now().UTC().UnixNano())
	altBoundary := fmt.Sprintf("localitas-alt-%d", time.Now().UTC().UnixNano())

	msgID := fmt.Sprintf("<%d.%s@localitas>", time.Now().UTC().UnixNano(), account.Email)
	headers := "From: " + from + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n"
	if len(cc) > 0 {
		headers += "Cc: " + strings.Join(cc, ", ") + "\r\n"
	}
	headers += "Message-ID: " + msgID + "\r\n"
	if inReplyTo != "" {
		headers += "In-Reply-To: " + inReplyTo + "\r\n" +
			"References: " + inReplyTo + "\r\n"
	}
	headers += "Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n" +
		"\r\n"

	var msg strings.Builder
	msg.WriteString(headers)

	msg.WriteString("--" + boundary + "\r\n")
	if bodyHTML != "" {
		msg.WriteString("Content-Type: multipart/alternative; boundary=\"" + altBoundary + "\"\r\n\r\n")
		msg.WriteString("--" + altBoundary + "\r\n")
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		msg.WriteString(body + "\r\n")
		msg.WriteString("--" + altBoundary + "\r\n")
		msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		msg.WriteString(bodyHTML + "\r\n")
		msg.WriteString("--" + altBoundary + "--\r\n")
	} else {
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		msg.WriteString(body + "\r\n")
	}

	for _, att := range attachments {
		msg.WriteString("--" + boundary + "\r\n")
		ct := att.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		msg.WriteString("Content-Type: " + ct + "\r\n")
		msg.WriteString("Content-Disposition: attachment; filename=\"" + att.Filename + "\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		encoded := base64Encode(att.Data)
		msg.WriteString(encoded + "\r\n")
	}
	msg.WriteString("--" + boundary + "--\r\n")

	allRecipients := append(append(to, cc...), bcc...)
	var auth smtp.Auth
	if account.NeedsOAuth() && account.AccessToken != "" {
		auth = &xoauth2SMTPAuth{email: account.Email, token: account.AccessToken}
	} else {
		auth = smtp.PlainAuth("", account.Username, account.Password, account.SMTPHost)
	}
	return smtp.SendMail(addr, auth, from, allRecipients, []byte(msg.String()))
}

func base64Encode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	var lines strings.Builder
	for i := 0; i < len(encoded); i += lineLen {
		end := i + lineLen
		if end > len(encoded) {
			end = len(encoded)
		}
		lines.WriteString(encoded[i:end])
		lines.WriteString("\r\n")
	}
	return lines.String()
}

func normalizeSubject(subject string) string {
	s := strings.ToLower(strings.TrimSpace(subject))
	for {
		changed := false
		for _, prefix := range []string{"re:", "fwd:", "fw:", "re[", "fwd["} {
			if strings.HasPrefix(s, prefix) {
				if prefix == "re[" || prefix == "fwd[" {
					if idx := strings.Index(s, "]:"); idx >= 0 {
						s = strings.TrimSpace(s[idx+2:])
						changed = true
					}
				} else {
					s = strings.TrimSpace(s[len(prefix):])
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return s
}

func shouldSyncFolder(name string) bool {
	lower := strings.ToLower(name)
	return lower == "inbox" ||
		strings.Contains(lower, "sent") ||
		strings.Contains(lower, "draft") ||
		strings.Contains(lower, "starred") ||
		strings.Contains(lower, "important")
}

func syncFolder(ctx context.Context, c *imapclient.Client, store *Store, accountID string, folder *Folder, maxMessages uint32, syncCfg *SyncConfig) (int, error) {
	activeFilters, _ := store.ListActiveFilters(ctx, accountID)

	mbox, err := c.Select(folder.RemoteName, true)
	if err != nil {
		return 0, fmt.Errorf("select %s: %w", folder.RemoteName, err)
	}

	store.UpdateFolderCounts(ctx, folder.ID, int(mbox.Messages), int(mbox.Unseen))

	if mbox.Messages == 0 {
		return 0, nil
	}

	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		section.FetchItem(),
	}

	var uidSet *imap.SeqSet
	useUID := false

	if folder.LastSyncedUID > 0 {
		uidSet = new(imap.SeqSet)
		uidSet.AddRange(folder.LastSyncedUID+1, 0)
		useUID = true
	} else {
		uidSet = new(imap.SeqSet)
		from := uint32(1)
		if mbox.Messages > maxMessages {
			from = mbox.Messages - maxMessages + 1
		}
		uidSet.AddRange(from, mbox.Messages)
	}

	messages := make(chan *imap.Message, 10)
	if useUID {
		go func() { c.UidFetch(uidSet, items, messages) }()
	} else {
		go func() { c.Fetch(uidSet, items, messages) }()
	}

	newCount := 0
	var highestUID uint32
	for msg := range messages {
		if msg.Uid > highestUID {
			highestUID = msg.Uid
		}
		if msg.Envelope == nil {
			continue
		}

		env := msg.Envelope
		fromName, fromAddr := "", ""
		if len(env.From) > 0 {
			fromName = env.From[0].PersonalName
			fromAddr = env.From[0].Address()
		}

		toAddrs := ""
		for i, a := range env.To {
			if i > 0 {
				toAddrs += ", "
			}
			if a.PersonalName != "" {
				toAddrs += a.PersonalName + " <" + a.Address() + ">"
			} else {
				toAddrs += a.Address()
			}
		}

		bodyText, bodyHTML, hasAttach := "", "", false
		unsubscribeURL := ""
		unsubscribePost := false
		var attachments []attachmentData
		for _, body := range msg.Body {
			rawBytes, _ := io.ReadAll(body)
			headerEnd := bytes.Index(rawBytes, []byte("\r\n\r\n"))
			if headerEnd < 0 {
				headerEnd = bytes.Index(rawBytes, []byte("\n\n"))
			}
			if headerEnd > 0 {
				headerStr := string(rawBytes[:headerEnd])
				unsubVal := extractHeaderValue(headerStr, "List-Unsubscribe")
				if unsubVal != "" && unsubscribeURL == "" {
					unsubscribeURL = parseUnsubscribeURL(unsubVal)
				}
				if extractHeaderValue(headerStr, "List-Unsubscribe-Post") != "" {
					unsubscribePost = true
				}
			}
			mr, err := mail.CreateReader(bytes.NewReader(rawBytes))
			if err != nil {
				continue
			}
			if unsubscribeURL == "" {
				if listUnsub := mr.Header.Get("List-Unsubscribe"); listUnsub != "" {
					unsubscribeURL = parseUnsubscribeURL(listUnsub)
				}
			}
			if !unsubscribePost && mr.Header.Get("List-Unsubscribe-Post") != "" {
				unsubscribePost = true
			}
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				switch h := part.Header.(type) {
				case *mail.InlineHeader:
					ct, _, _ := h.ContentType()
					b, _ := io.ReadAll(part.Body)
					if strings.HasPrefix(ct, "text/plain") && bodyText == "" {
						bodyText = string(b)
					} else if strings.HasPrefix(ct, "text/html") && bodyHTML == "" {
						bodyHTML = string(b)
					}
				case *mail.AttachmentHeader:
					hasAttach = true
					attachFilename, _ := h.Filename()
					attachCT, _, _ := h.ContentType()
					attachData, _ := io.ReadAll(part.Body)
					if attachFilename != "" && len(attachData) > 0 {
						attachments = append(attachments, attachmentData{
							Filename:    attachFilename,
							ContentType: attachCT,
							Data:        attachData,
						})
					}
				}
			}
		}

		if unsubscribeURL == "" && bodyHTML != "" {
			unsubscribeURL = findUnsubscribeLinkInHTML(bodyHTML)
		}

		snippet := bodyText
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}

		date := env.Date.Unix()
		if date <= 0 {
			date = time.Now().UTC().Unix()
		}

		inReplyTo := env.InReplyTo
		threadID := ""
		if inReplyTo != "" {
			threadID = resolveThreadID(ctx, store, accountID, inReplyTo)
		}

		isRead := false
		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				isRead = true
				break
			}
		}

		skipInsert := false
		if len(activeFilters) > 0 {
			candidate := &Email{
				Subject:        env.Subject,
				FromName:       fromName,
				FromAddress:    fromAddr,
				ToAddresses:    toAddrs,
				BodyText:       bodyText,
				IsRead:         isRead,
				IsStarred:      false,
				HasAttachments: hasAttach,
			}
			actions := ApplyFilters(activeFilters, candidate)
			for _, act := range actions {
				switch act.Type {
				case "delete":
					skipInsert = true
				case "archive":
					isRead = true
				case "mark_read":
					isRead = true
				case "mark_unread":
					isRead = false
				case "star":
					candidate.IsStarred = true
				}
			}
		}

		if skipInsert {
			continue
		}

		err := store.InsertEmail(ctx, accountID, folder.ID, env.MessageId, env.Subject,
			fromName, fromAddr, toAddrs, snippet, bodyText, bodyHTML, date, msg.Uid, hasAttach, isRead, threadID, inReplyTo, unsubscribeURL, unsubscribePost)
		if err == nil {
			newCount++
			if len(attachments) > 0 && syncCfg != nil && syncCfg.CoreURL != "" {
				go saveEmailAttachments(ctx, store, syncCfg, accountID, env.MessageId, attachments)
			}
		}
	}

	if highestUID > 0 {
		store.UpdateLastSyncedUID(ctx, folder.ID, highestUID)
	}

	if useUID {
		syncFolderFlags(ctx, c, store, accountID, folder)
	}

	return newCount, nil
}

func saveEmailAttachments(ctx context.Context, store *Store, cfg *SyncConfig, accountID, messageID string, atts []attachmentData) {
	var emailID string
	store.db.QueryRowContext(ctx, "SELECT id FROM emails WHERE account_id = ? AND message_id = ?", accountID, messageID).Scan(&emailID)
	if emailID == "" {
		return
	}
	for _, att := range atts {
		if cfg.CoreURL != "" && scanAttachment(ctx, cfg, att) {
			store.MarkDangerous(ctx, emailID)
			log.Printf("⚠️  Dangerous attachment detected: %s in email %s", att.Filename, emailID)
			continue
		}

		storagePath, err := SaveAttachmentToManagedFS(ctx, cfg.CoreURL, cfg.AuthToken, emailID, att.Filename, att.Data)
		if err != nil {
			log.Printf("⚠️  Failed to save attachment %s: %v", att.Filename, err)
			continue
		}
		store.InsertAttachment(ctx, emailID, accountID, att.Filename, att.ContentType, storagePath, int64(len(att.Data)))
	}
}

func scanAttachment(ctx context.Context, cfg *SyncConfig, att attachmentData) bool {
	scanURL := cfg.CoreURL + "/apps/ext/antivirus/api/scan"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", att.Filename)
	if err != nil {
		return false
	}
	part.Write(att.Data)
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", scanURL, body)
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("⚠️  Antivirus scan unavailable for %s: %v", att.Filename, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var result struct {
		Verdict string `json:"verdict"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Verdict == "infected"
}

func syncFolderFlags(ctx context.Context, c *imapclient.Client, store *Store, accountID string, folder *Folder) {
	if folder.LastSyncedUID == 0 {
		return
	}
	startUID := folder.LastSyncedUID
	if startUID > 200 {
		startUID -= 200
	} else {
		startUID = 1
	}
	uidSet := new(imap.SeqSet)
	uidSet.AddRange(startUID, folder.LastSyncedUID)

	messages := make(chan *imap.Message, 50)
	go func() { c.UidFetch(uidSet, []imap.FetchItem{imap.FetchFlags, imap.FetchUid}, messages) }()

	for msg := range messages {
		isRead := false
		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				isRead = true
				break
			}
		}
		readInt := 0
		if isRead {
			readInt = 1
		}
		store.db.ExecContext(ctx, "UPDATE emails SET is_read = ? WHERE account_id = ? AND folder_id = ? AND uid = ?",
			readInt, accountID, folder.ID, msg.Uid)
	}
}
