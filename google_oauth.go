package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/localitas/localitas-go"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	gmailScope     = "https://mail.google.com/"
)

func GoogleAuthRedirectURL(clientID, redirectURI, state string) string {
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {gmailScope},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	return googleAuthURL + "?" + params.Encode()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func ExchangeGoogleCode(ctx context.Context, code, clientID, clientSecret, redirectURI string) (*tokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func RefreshGoogleToken(ctx context.Context, refreshToken, clientID, clientSecret string) (*tokenResponse, error) {
	data := url.Values{
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func (s *Store) SaveOAuthTokens(ctx context.Context, accountID, accessToken, refreshToken string, expiresIn int) error {
	now := time.Now().UTC().Unix()
	expiry := now + int64(expiresIn)
	encAccess, _ := client.Encrypt(accessToken)
	encRefresh, _ := client.Encrypt(refreshToken)
	_, err := s.db.ExecContext(ctx,
		"UPDATE accounts SET access_token = ?, refresh_token = ?, token_expiry = ?, updated_at = ? WHERE id = ?",
		encAccess, encRefresh, expiry, now, accountID)
	return err
}

func (s *Store) GetAccountWithTokens(ctx context.Context, id string) (*Account, error) {
	a, err := s.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	var accessToken, refreshToken string
	var tokenExpiry int64
	s.db.QueryRowContext(ctx, "SELECT COALESCE(access_token,''), COALESCE(refresh_token,''), COALESCE(token_expiry,0) FROM accounts WHERE id = ?", id).
		Scan(&accessToken, &refreshToken, &tokenExpiry)
	a.AccessToken, _ = client.Decrypt(accessToken)
	a.RefreshToken, _ = client.Decrypt(refreshToken)
	a.TokenExpiry = tokenExpiry
	return a, nil
}

func EnsureValidToken(ctx context.Context, store *Store, account *Account) error {
	if !account.NeedsOAuth() {
		return nil
	}
	if account.HasValidToken() {
		return nil
	}
	if account.RefreshToken == "" {
		return fmt.Errorf("no refresh token — re-authorize with Google")
	}
	tok, err := RefreshGoogleToken(ctx, account.RefreshToken, account.OAuthClientID, account.OAuthClientSecret)
	if err != nil {
		return fmt.Errorf("token refresh: %w", err)
	}
	account.AccessToken = tok.AccessToken
	account.TokenExpiry = time.Now().UTC().Unix() + int64(tok.ExpiresIn)

	refreshToSave := account.RefreshToken
	if tok.RefreshToken != "" {
		refreshToSave = tok.RefreshToken
		account.RefreshToken = tok.RefreshToken
	}
	return store.SaveOAuthTokens(ctx, account.ID, tok.AccessToken, refreshToSave, tok.ExpiresIn)
}
