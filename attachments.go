package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	client "github.com/localitas/localitas-go"
)

// httpClient is a single pooled client shared across attachment save/get and the
// antivirus scan, created once instead of per attachment (saveEmailAttachments
// loops over every attachment of every synced email). No hard Timeout; each call
// bounds itself with a context deadline so save/get and scan keep their own
// limits while sharing the connection pool. http.Client is concurrency-safe.
var httpClient = &http.Client{}

const (
	attachmentIOTimeout   = 30 * time.Second
	attachmentScanTimeout = 60 * time.Second
	oauthTimeout          = 10 * time.Second
)

// StoredFile is the managed file an attachment is kept in, in the
// storage of the token's user, held by ID.
type StoredFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func filesClient(coreURL, token string) *client.Client {
	return client.New(coreURL).WithHTTPClient(httpClient).WithToken(token)
}

// attachmentFolder is the managed folder holding an email's attachments.
func attachmentFolder(emailID string) string {
	return "email-attachments/" + emailID
}

// attachmentName is filename made a valid file name.
func attachmentName(filename string) string {
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < ' ' {
			return '_'
		}
		return r
	}, strings.TrimSpace(filename))
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

// SaveAttachmentToManagedFS keeps data as a file in the email's folder,
// made as needed, under a name the filesystem frees for it, so two
// attachments of one name are both kept.
func SaveAttachmentToManagedFS(ctx context.Context, coreURL, token, emailID, filename string, data []byte) (*StoredFile, error) {
	ctx, cancel := context.WithTimeout(ctx, attachmentIOTimeout)
	defer cancel()
	q := url.Values{"folder": {attachmentFolder(emailID)}, "name": {attachmentName(filename)}, "conflict": {"rename"}}
	var out StoredFile
	if err := filesClient(coreURL, token).Send(ctx, "POST", "/apps/filesystem/api/nodes?"+q.Encode(), "application/octet-stream", bytes.NewReader(data), &out); err != nil {
		return nil, fmt.Errorf("store attachment %s: %w", filename, err)
	}
	return &out, nil
}

// ResolveAttachmentPath returns the file at path in the token user's
// managed storage, for attachments stored by path before IDs.
func ResolveAttachmentPath(ctx context.Context, coreURL, token, path string) (*StoredFile, error) {
	ctx, cancel := context.WithTimeout(ctx, attachmentIOTimeout)
	defer cancel()
	var out StoredFile
	if err := filesClient(coreURL, token).Do(ctx, "POST", "/apps/filesystem/api/nodes/resolve", struct {
		Path string `json:"path"`
	}{path}, &out); err != nil {
		return nil, fmt.Errorf("resolve attachment %s: %w", path, err)
	}
	return &out, nil
}

// DownloadAttachment streams managed file fileID to w, from whichever node
// holds it.
func DownloadAttachment(ctx context.Context, coreURL, token, fileID string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, attachmentIOTimeout)
	defer cancel()
	_, err := filesClient(coreURL, token).Download(ctx, "/apps/filesystem/managed/"+url.PathEscape(fileID), w)
	return err
}

// isNotFound reports whether err is core answering 404.
func isNotFound(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
