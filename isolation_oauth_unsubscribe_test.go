package email

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

// oauthAccount gives user a Gmail OAuth account and returns its ID.
func oauthAccount(t *testing.T, env *testEnv, user string) string {
	t.Helper()
	acct, err := env.store.CreateAccount(context.Background(), user, user+" gmail", user+"@gmail.example.com", "gmail-oauth", "imap.gmail.com", 993, "smtp.gmail.com", 587, user, "", "client-id", "client-secret", true, "")
	if err != nil {
		t.Fatal(err)
	}
	return acct.ID
}

func TestOAuthFlowOnlyAuthorizesTheCallersAccount(t *testing.T) {
	env := newTestEnv(t)
	aliceGmail := oauthAccount(t, env, "alice")
	oauthAccount(t, env, "bob")

	expectStatus(t, env.as("bob", "GET", "/api/oauth/"+aliceGmail+"/start", nil), http.StatusNotFound, "bob starts OAuth for alice's account")

	w := env.as("alice", "GET", "/api/oauth/"+aliceGmail+"/start", nil)
	expectStatus(t, w, http.StatusOK, "alice starts OAuth for her account")
	start := decode[map[string]string](t, w)
	authURL, err := url.Parse(start["auth_url"])
	if err != nil {
		t.Fatal(err)
	}
	state := authURL.Query().Get("state")
	if state == "" || state == aliceGmail {
		t.Fatalf("state %q must be sealed, not the bare account ID", state)
	}

	callback := func(state string) string {
		return "/api/oauth/callback?code=code&state=" + url.QueryEscape(state)
	}
	expectStatus(t, env.as("bob", "GET", callback(state), nil), http.StatusNotFound, "bob completes alice's flow")
	expectStatus(t, env.as("bob", "GET", callback(aliceGmail), nil), http.StatusBadRequest, "bob names alice's account as a bare state")
	expectStatus(t, env.as("bob", "GET", callback("alice\n"+aliceGmail), nil), http.StatusBadRequest, "bob forges a plaintext state")

	bobState, err := sealOAuthState("bob", aliceGmail)
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, env.as("bob", "GET", callback(bobState), nil), http.StatusNotFound, "bob's own state naming alice's account")

	acct, err := env.store.GetAccountWithTokens(context.Background(), "alice", aliceGmail)
	if err != nil {
		t.Fatal(err)
	}
	if acct.AccessToken != "" || acct.RefreshToken != "" {
		t.Fatal("alice's tokens were written by bob's callbacks")
	}
	if err := env.store.SaveOAuthTokens(context.Background(), "bob", aliceGmail, "a", "r", 3600); err != ErrNotFound {
		t.Fatalf("bob saves tokens on alice's account: %v, want ErrNotFound", err)
	}
}

// listUnsubscribe serves an unsubscribe endpoint and counts its hits.
func listUnsubscribe(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestUnsubscribeUsesOnlyTheEmailsOwnTarget(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	supplied, suppliedHits := listUnsubscribe(t)
	own, ownHits := listUnsubscribe(t)

	body := map[string]string{"url": supplied.URL + "/internal"}
	expectStatus(t, env.as("alice", "POST", "/api/emails/"+env.alice.email.ID+"/unsubscribe", body), http.StatusBadRequest, "unsubscribe from an email without List-Unsubscribe")

	listed := env.addEmail(env.alice, 2, "newsletter", own.URL+"/unsub")
	expectStatus(t, env.as("bob", "POST", "/api/emails/"+listed.ID+"/unsubscribe", body), http.StatusNotFound, "bob unsubscribes alice's email")
	if ownHits.Load() != 0 {
		t.Fatal("bob's request reached alice's unsubscribe target")
	}

	expectStatus(t, env.as("alice", "POST", "/api/emails/"+listed.ID+"/unsubscribe", body), http.StatusOK, "alice unsubscribes")
	if suppliedHits.Load() != 0 {
		t.Fatalf("the caller-supplied URL was fetched %d times", suppliedHits.Load())
	}
	if ownHits.Load() != 1 {
		t.Fatalf("the email's own target was fetched %d times, want 1", ownHits.Load())
	}
	trash, err := env.store.ListDeletedEmails(ctx, "alice", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 1 || trash[0].ID != listed.ID {
		t.Fatalf("alice's trash after unsubscribing = %+v", trash)
	}
}
