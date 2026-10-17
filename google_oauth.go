package email

import (
	"context"
	"encoding/json"
	"errors"
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
	ctx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
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
	ctx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
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

// SaveOAuthTokens stores the tokens of userID's account accountID.
func (s *Store) SaveOAuthTokens(ctx context.Context, userID, accountID, accessToken, refreshToken string, expiresIn int) error {
	now := time.Now().UTC().Unix()
	expiry := now + int64(expiresIn)
	encAccess, err := client.Encrypt(accessToken)
	if err != nil {
		return fmt.Errorf("encrypt access_token: %w", err)
	}
	encRefresh, err := client.Encrypt(refreshToken)
	if err != nil {
		return fmt.Errorf("encrypt refresh_token: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE accounts SET access_token = ?, refresh_token = ?, token_expiry = ?, updated_at = ? WHERE id = ? AND user_id = ?",
		encAccess, encRefresh, expiry, now, accountID, userID)
	return requireOneRow(res, err)
}

// GetAccountWithTokens returns userID's account id with its decrypted
// secrets and OAuth tokens.
func (s *Store) GetAccountWithTokens(ctx context.Context, userID, id string) (*Account, error) {
	a, err := s.GetAccount(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	var accessToken, refreshToken string
	var tokenExpiry int64
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(access_token,''), COALESCE(refresh_token,''), COALESCE(token_expiry,0) FROM accounts WHERE id = ? AND user_id = ?", id, userID).
		Scan(&accessToken, &refreshToken, &tokenExpiry); err != nil {
		return nil, fmt.Errorf("read tokens: %w", err)
	}
	if a.AccessToken, err = client.Decrypt(accessToken); err != nil {
		return nil, fmt.Errorf("decrypt access_token: %w", err)
	}
	if a.RefreshToken, err = client.Decrypt(refreshToken); err != nil {
		return nil, fmt.Errorf("decrypt refresh_token: %w", err)
	}
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
	return store.SaveOAuthTokens(ctx, account.UserID, account.ID, tok.AccessToken, refreshToSave, tok.ExpiresIn)
}

// sealOAuthState binds an OAuth flow to the user who started it and the
// account it authorizes. It is encrypted and authenticated, so a callback
// cannot be pointed at an account its caller did not start the flow for.
func sealOAuthState(userID, accountID string) (string, error) {
	return client.Encrypt(userID + "\n" + accountID)
}

// openOAuthState returns the user and account sealOAuthState bound. A state
// that was not sealed by this app is refused: Decrypt would pass plaintext
// through unchanged.
func openOAuthState(state string) (userID, accountID string, err error) {
	if !strings.HasPrefix(state, "enc:") {
		return "", "", errBadOAuthState
	}
	plain, err := client.Decrypt(state)
	if err != nil {
		return "", "", errBadOAuthState
	}
	userID, accountID, ok := strings.Cut(plain, "\n")
	if !ok || userID == "" || accountID == "" {
		return "", "", errBadOAuthState
	}
	return userID, accountID, nil
}

var errBadOAuthState = errors.New("invalid OAuth state")
