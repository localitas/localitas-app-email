package email

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIRequiresVerifiedUser(t *testing.T) {
	env := newTestEnv(t)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/accounts"},
		{"PUT", "/api/accounts/" + env.alice.account.ID},
		{"GET", "/api/emails/" + env.alice.email.ID},
		{"GET", "/api/search?q=quarterly"},
		{"GET", "/api/drafts"},
		{"GET", "/api/filters?account_id=" + env.alice.account.ID},
		{"GET", "/api/oauth/callback?code=x&state=y"},
	} {
		w := env.do(route.method, route.path, "", "", nil)
		expectStatus(t, w, http.StatusUnauthorized, route.method+" "+route.path+" without identity")
	}

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+identityToken("alice", "write")+"x")
	w := httptest.NewRecorder()
	env.mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusUnauthorized, "tampered identity")
}

func TestMutationsNeedWriteScope(t *testing.T) {
	env := newTestEnv(t)
	w := env.do("POST", "/api/emails/"+env.alice.email.ID+"/star", "alice", "read", nil)
	expectStatus(t, w, http.StatusForbidden, "star with read scope")
	w = env.do("GET", "/api/emails/"+env.alice.email.ID, "alice", "read", nil)
	expectStatus(t, w, http.StatusOK, "read email with read scope")
}

func TestAccountsAreOwnedByTheirUser(t *testing.T) {
	env := newTestEnv(t)
	alice := env.alice.account

	got := decode[[]Account](t, env.as("bob", "GET", "/api/accounts", nil))
	if len(got) != 1 || got[0].ID != env.bob.account.ID {
		t.Fatalf("bob lists %+v, want only his own account", got)
	}

	repoint := map[string]any{"name": "x", "email": "alice@example.com", "imap_host": "evil.example.com", "imap_port": 993, "smtp_host": "evil.example.com", "smtp_port": 587, "username": "alice"}
	expectStatus(t, env.as("bob", "PUT", "/api/accounts/"+alice.ID, repoint), http.StatusNotFound, "bob repoints alice's account")
	expectStatus(t, env.as("bob", "PUT", "/api/accounts/no-such-account", repoint), http.StatusNotFound, "bob updates a missing account")
	stored, err := env.store.GetAccount(context.Background(), "alice", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.IMAPHost != "127.0.0.1" || stored.Password != "alice-password" {
		t.Fatalf("alice's account changed: host %q", stored.IMAPHost)
	}

	expectStatus(t, env.as("bob", "POST", "/api/accounts/"+alice.ID+"/sync", nil), http.StatusNotFound, "bob syncs alice's account")
	expectStatus(t, env.as("bob", "POST", "/api/folders/"+env.alice.folder.ID+"/sync", nil), http.StatusNotFound, "bob syncs alice's folder")
	expectStatus(t, env.as("bob", "GET", "/api/folders?account_id="+alice.ID, nil), http.StatusNotFound, "bob lists alice's folders")
	expectStatus(t, env.as("bob", "DELETE", "/api/accounts/"+alice.ID, nil), http.StatusNotFound, "bob deletes alice's account")
	if _, err := env.store.GetEmail(context.Background(), "alice", env.alice.email.ID); err != nil {
		t.Fatalf("alice's mail after bob's delete: %v", err)
	}

	w := env.as("alice", "POST", "/api/accounts/"+alice.ID+"/sync", nil)
	expectStatus(t, w, http.StatusInternalServerError, "alice syncs her account against a closed port")
	folders := decode[[]Folder](t, env.as("alice", "GET", "/api/folders?account_id="+alice.ID, nil))
	if len(folders) != 1 {
		t.Fatalf("alice sees %d folders, want 1", len(folders))
	}
	repoint["imap_host"] = "imap.example.com"
	expectStatus(t, env.as("alice", "PUT", "/api/accounts/"+alice.ID, repoint), http.StatusOK, "alice updates her account")
	expectStatus(t, env.as("alice", "DELETE", "/api/accounts/"+alice.ID, nil), http.StatusOK, "alice deletes her account")
	if _, err := env.store.GetEmail(context.Background(), "alice", env.alice.email.ID); err != ErrNotFound {
		t.Fatalf("alice's mail after deleting her account: %v, want ErrNotFound", err)
	}
	if _, err := env.store.GetEmail(context.Background(), "bob", env.bob.email.ID); err != nil {
		t.Fatalf("bob's mail after alice deleted hers: %v", err)
	}
}

func TestSyncAllIsSystemOnly(t *testing.T) {
	env := newTestEnv(t)
	expectStatus(t, env.as("alice", "POST", "/api/sync-all", nil), http.StatusForbidden, "user calls sync-all")
	expectStatus(t, env.do("POST", "/api/sync-all", "", "", nil), http.StatusForbidden, "anonymous sync-all")

	req := httptest.NewRequest("POST", "/api/sync-all", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	w := httptest.NewRecorder()
	env.mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK, "core calls sync-all")
	result := decode[SyncAllResult](t, w)
	if result.AccountsSynced != 2 || len(result.Errors) != 2 {
		t.Fatalf("sync-all = %+v, want both users' accounts attempted (each failing on the closed port)", result)
	}
}
