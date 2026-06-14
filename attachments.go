package email

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func SaveAttachmentToManagedFS(ctx context.Context, coreURL, token, emailID, filename string, data []byte) (string, error) {
	storagePath := fmt.Sprintf("email-attachments/%s/%s", emailID, filename)
	webdavURL := fmt.Sprintf("%s/apps/filesystem/webdav/managed/%s", coreURL, url.PathEscape(storagePath))

	dirURL := fmt.Sprintf("%s/apps/filesystem/webdav/managed/email-attachments/%s/", coreURL, emailID)
	mkReq, _ := http.NewRequestWithContext(ctx, "MKCOL", dirURL, nil)
	if token != "" {
		mkReq.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	mkResp, _ := client.Do(mkReq)
	if mkResp != nil {
		mkResp.Body.Close()
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", webdavURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("webdav put failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 && resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("webdav put %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	return storagePath, nil
}

func GetAttachmentFromManagedFS(ctx context.Context, coreURL, token, storagePath string) ([]byte, string, error) {
	webdavURL := fmt.Sprintf("%s/apps/filesystem/webdav/managed/%s", coreURL, url.PathEscape(storagePath))

	req, err := http.NewRequestWithContext(ctx, "GET", webdavURL, nil)
	if err != nil {
		return nil, "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("webdav get %d", resp.StatusCode)
	}

	data, _ := io.ReadAll(resp.Body)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}
