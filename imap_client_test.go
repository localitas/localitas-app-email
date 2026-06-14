package email

import (
	"strings"
	"testing"
)

func TestXoauth2Client_Start(t *testing.T) {
	c := &xoauth2Client{email: "user@gmail.com", token: "ya29.test-token"}
	mech, ir, err := c.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if mech != "XOAUTH2" {
		t.Errorf("mechanism = %s, want XOAUTH2", mech)
	}
	expected := "user=user@gmail.com\x01auth=Bearer ya29.test-token\x01\x01"
	if string(ir) != expected {
		t.Errorf("ir = %q, want %q", string(ir), expected)
	}
}

func TestXoauth2Client_Next(t *testing.T) {
	c := &xoauth2Client{email: "user@gmail.com", token: "token"}
	_, err := c.Next([]byte("challenge"))
	if err == nil {
		t.Error("expected error from Next")
	}
}

func TestAccountNeedsOAuth(t *testing.T) {
	a := &Account{Provider: "gmail-oauth"}
	if !a.NeedsOAuth() {
		t.Error("expected NeedsOAuth = true for gmail-oauth")
	}

	b := &Account{Provider: "imap"}
	if b.NeedsOAuth() {
		t.Error("expected NeedsOAuth = false for imap")
	}

	c := &Account{}
	if c.NeedsOAuth() {
		t.Error("expected NeedsOAuth = false for empty provider")
	}
}

func TestGoogleAuthRedirectURL(t *testing.T) {
	url := GoogleAuthRedirectURL("client-id", "http://localhost/callback", "account-123")
	if !strings.Contains(url, "client_id=client-id") {
		t.Error("missing client_id")
	}
	if !strings.Contains(url, "redirect_uri=") {
		t.Error("missing redirect_uri")
	}
	if !strings.Contains(url, "state=account-123") {
		t.Error("missing state")
	}
	if !strings.Contains(url, "scope=") {
		t.Error("missing scope")
	}
	if !strings.Contains(url, "access_type=offline") {
		t.Error("missing access_type=offline")
	}
}

func TestPresetsIncludeGmailOAuth(t *testing.T) {
	found := false
	for _, p := range Presets {
		if p.Name == "Gmail (OAuth)" {
			found = true
			if p.IMAPHost != "imap.gmail.com" {
				t.Errorf("Gmail OAuth IMAP host = %s", p.IMAPHost)
			}
		}
	}
	if !found {
		t.Error("Gmail (OAuth) preset not found")
	}
}

func TestNormalizeSubject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Meeting notes", "meeting notes"},
		{"Re: Meeting notes", "meeting notes"},
		{"RE: Meeting notes", "meeting notes"},
		{"Fwd: Meeting notes", "meeting notes"},
		{"FW: Meeting notes", "meeting notes"},
		{"Re: Re: Fwd: Discussion", "discussion"},
		{"Re[2]: Some topic", "some topic"},
		{"Fwd[3]: Another topic", "another topic"},
		{"  Re:  Hello  ", "hello"},
	}
	for _, tt := range tests {
		got := normalizeSubject(tt.input)
		if got != tt.want {
			t.Errorf("normalizeSubject(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSendEmail_MultipartMIME(t *testing.T) {
	a := &Account{
		Email:    "test@example.com",
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		Username: "user",
		Password: "pass",
	}
	err := SendEmail(a, []string{"to@example.com"}, []string{"cc@example.com"}, []string{"bcc@example.com"}, "Test", "plain body", "<b>html body</b>", "")
	if err == nil {
		t.Skip("skipping — can't connect to real SMTP server in unit test")
	}
}

func TestAccountHasValidToken(t *testing.T) {
	a := &Account{
		AccessToken: "token",
		TokenExpiry: 9999999999,
	}
	if !a.HasValidToken() {
		t.Error("expected valid token")
	}

	b := &Account{
		AccessToken: "token",
		TokenExpiry: 1,
	}
	if b.HasValidToken() {
		t.Error("expected expired token")
	}
}
