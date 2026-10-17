package email

import (
	"context"
	"net/http"
	"testing"
)

func TestDraftsAreOwnedByTheirUser(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	draft := env.alice.draft

	drafts := decode[[]Draft](t, env.as("bob", "GET", "/api/drafts", nil))
	if len(drafts) != 1 || drafts[0].ID != env.bob.draft.ID {
		t.Fatalf("bob lists %+v, want only his draft", drafts)
	}
	expectStatus(t, env.as("bob", "GET", "/api/drafts/"+draft.ID, nil), http.StatusNotFound, "bob reads alice's draft")
	expectStatus(t, env.as("bob", "DELETE", "/api/drafts/"+draft.ID, nil), http.StatusNotFound, "bob deletes alice's draft")

	overwrite := map[string]string{"id": draft.ID, "account_id": env.bob.account.ID, "subject": "hijacked"}
	expectStatus(t, env.as("bob", "POST", "/api/drafts", overwrite), http.StatusNotFound, "bob overwrites alice's draft")
	intoAlice := map[string]string{"account_id": env.alice.account.ID, "subject": "planted"}
	expectStatus(t, env.as("bob", "POST", "/api/drafts", intoAlice), http.StatusNotFound, "bob saves a draft into alice's account")
	got, err := env.store.GetDraft(ctx, "alice", draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != draft.Subject || got.AccountID != env.alice.account.ID {
		t.Fatalf("alice's draft changed: %+v", got)
	}
	if mine, _ := env.store.ListDrafts(ctx, "alice"); len(mine) != 1 {
		t.Fatalf("alice has %d drafts, want 1", len(mine))
	}

	update := map[string]string{"id": draft.ID, "account_id": env.alice.account.ID, "subject": "revised"}
	if saved := decode[Draft](t, env.as("alice", "POST", "/api/drafts", update)); saved.Subject != "revised" {
		t.Fatalf("alice's save = %+v", saved)
	}
	if got := decode[Draft](t, env.as("alice", "GET", "/api/drafts/"+draft.ID, nil)); got.Subject != "revised" {
		t.Fatalf("alice reads %+v", got)
	}
	expectStatus(t, env.as("alice", "POST", "/api/drafts", map[string]string{"subject": "no account"}), http.StatusBadRequest, "draft without account")
	expectStatus(t, env.as("alice", "DELETE", "/api/drafts/"+draft.ID, nil), http.StatusOK, "alice deletes her draft")
}

func TestFiltersAreOwnedByTheirUser(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	filter := env.alice.filter
	rule := map[string]any{"rule": "from:spam => archive", "is_active": false}

	expectStatus(t, env.as("bob", "GET", "/api/filters?account_id="+env.alice.account.ID, nil), http.StatusNotFound, "bob lists alice's filters")
	expectStatus(t, env.as("bob", "POST", "/api/filters", map[string]string{"account_id": env.alice.account.ID, "rule": "from:x => star"}), http.StatusNotFound, "bob adds a filter to alice's account")
	expectStatus(t, env.as("bob", "PUT", "/api/filters/"+filter.ID, rule), http.StatusNotFound, "bob rewrites alice's filter")
	expectStatus(t, env.as("bob", "DELETE", "/api/filters/"+filter.ID, nil), http.StatusNotFound, "bob deletes alice's filter")
	filters, err := env.store.ListFilters(ctx, "alice", env.alice.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(filters) != 1 || filters[0].Rule != filter.Rule || !filters[0].IsActive {
		t.Fatalf("alice's filters changed: %+v", filters)
	}

	expectStatus(t, env.as("alice", "POST", "/api/filters", map[string]string{"account_id": env.alice.account.ID, "rule": "not a rule"}), http.StatusBadRequest, "alice adds an invalid rule")
	expectStatus(t, env.as("alice", "PUT", "/api/filters/"+filter.ID, rule), http.StatusOK, "alice rewrites her filter")
	if list := decode[[]Filter](t, env.as("alice", "GET", "/api/filters?account_id="+env.alice.account.ID, nil)); len(list) != 1 || list[0].IsActive {
		t.Fatalf("alice's filters = %+v, want her rewritten, inactive filter", list)
	}
	expectStatus(t, env.as("alice", "DELETE", "/api/filters/"+filter.ID, nil), http.StatusOK, "alice deletes her filter")
}

func TestComposeOnlySendsFromTheCallersAccount(t *testing.T) {
	env := newTestEnv(t)
	msg := map[string]any{"account_id": env.alice.account.ID, "to": []string{"x@example.com"}, "subject": "hi", "body": "hi"}
	expectStatus(t, env.as("bob", "POST", "/api/compose", msg), http.StatusNotFound, "bob sends from alice's account")
	expectStatus(t, env.as("alice", "POST", "/api/compose", msg), http.StatusInternalServerError, "alice sends against a closed SMTP port")
}
