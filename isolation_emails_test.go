package email

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestEmailsAreOwnedByTheirUser(t *testing.T) {
	env := newTestEnv(t)
	id := env.alice.email.ID

	for _, route := range []struct{ method, path string }{
		{"GET", "/api/emails/" + id},
		{"POST", "/api/emails/" + id + "/star"},
		{"POST", "/api/emails/" + id + "/unread"},
		{"POST", "/api/emails/" + id + "/restore"},
		{"DELETE", "/api/emails/" + id},
		{"GET", "/api/emails/" + id + "/attachments"},
		{"GET", "/api/attachments/" + env.alice.attachment.ID + "/download"},
		{"GET", "/api/emails?folder_id=" + env.alice.folder.ID},
		{"GET", "/api/thread?thread_id=" + url.QueryEscape(env.alice.threadID)},
		{"GET", "/api/trash?account_id=" + env.alice.account.ID},
	} {
		expectStatus(t, env.as("bob", route.method, route.path, nil), http.StatusNotFound, "bob: "+route.method+" "+route.path)
	}
	expectStatus(t, env.as("bob", "GET", "/api/emails/no-such-email", nil), http.StatusNotFound, "bob reads a missing email")

	e, err := env.store.GetEmail(context.Background(), "alice", id)
	if err != nil {
		t.Fatal(err)
	}
	if e.IsStarred || e.IsRead {
		t.Fatalf("bob's requests changed alice's email: starred %v read %v", e.IsStarred, e.IsRead)
	}

	got := decode[Email](t, env.as("alice", "GET", "/api/emails/"+id, nil))
	if got.ID != id {
		t.Fatalf("alice reads %q, want %q", got.ID, id)
	}
	expectStatus(t, env.as("alice", "POST", "/api/emails/"+id+"/star", nil), http.StatusOK, "alice stars her email")
	expectStatus(t, env.as("alice", "POST", "/api/emails/"+id+"/unread", nil), http.StatusOK, "alice marks her email unread")
	if e, _ = env.store.GetEmail(context.Background(), "alice", id); !e.IsStarred || e.IsRead {
		t.Fatalf("alice's email: starred %v read %v, want starred and unread", e.IsStarred, e.IsRead)
	}
	if list := decode[[]Email](t, env.as("alice", "GET", "/api/emails?folder_id="+env.alice.folder.ID, nil)); len(list) != 1 {
		t.Fatalf("alice lists %d emails, want 1", len(list))
	}
	if thread := decode[[]Email](t, env.as("alice", "GET", "/api/thread?thread_id="+url.QueryEscape(env.alice.threadID), nil)); len(thread) != 1 {
		t.Fatalf("alice's thread has %d emails, want 1", len(thread))
	}
	if atts := decode[[]Attachment](t, env.as("alice", "GET", "/api/emails/"+id+"/attachments", nil)); len(atts) != 1 || atts[0].ID != env.alice.attachment.ID {
		t.Fatalf("alice's attachments = %+v", atts)
	}
	expectStatus(t, env.as("alice", "DELETE", "/api/emails/"+id, nil), http.StatusInternalServerError, "alice deletes her email against a closed IMAP port")
}

func TestSearchAndTrashOnlyShowTheCallersMail(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	hits := decode[[]Email](t, env.as("bob", "GET", "/api/search?q=quarterly", nil))
	if len(hits) != 1 || hits[0].ID != env.bob.email.ID {
		t.Fatalf("bob's search = %+v, want only his email", hits)
	}
	if hits := decode[[]Email](t, env.as("bob", "GET", "/api/search?q=alice", nil)); len(hits) != 0 {
		t.Fatalf("bob's search finds alice's mail: %+v", hits)
	}

	if err := env.store.SoftDelete(ctx, "alice", env.alice.email.ID); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SoftDelete(ctx, "alice", env.bob.email.ID); err != ErrNotFound {
		t.Fatalf("alice trashes bob's email: %v, want ErrNotFound", err)
	}
	if trash := decode[[]Email](t, env.as("bob", "GET", "/api/trash", nil)); len(trash) != 0 {
		t.Fatalf("bob's trash = %+v, want empty", trash)
	}
	if hits := decode[[]Email](t, env.as("bob", "GET", "/api/search?trash=1&q=quarterly", nil)); len(hits) != 0 {
		t.Fatalf("bob's trash search = %+v, want empty", hits)
	}
	trash := decode[[]Email](t, env.as("alice", "GET", "/api/trash", nil))
	if len(trash) != 1 || trash[0].ID != env.alice.email.ID {
		t.Fatalf("alice's trash = %+v", trash)
	}
	expectStatus(t, env.as("bob", "POST", "/api/emails/"+env.alice.email.ID+"/restore", nil), http.StatusNotFound, "bob restores alice's email")
	expectStatus(t, env.as("alice", "POST", "/api/emails/"+env.alice.email.ID+"/restore", nil), http.StatusOK, "alice restores her email")
}

func TestTrashSyncOnlyTouchesItsOwnAccount(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	shared := env.addEmail(env.bob, 2, "shared message", "")
	if err := env.store.SoftDeleteByMessageID(ctx, env.alice.account.ID, shared.MessageID); err != nil {
		t.Fatal(err)
	}
	e, err := env.store.GetEmail(ctx, "bob", shared.ID)
	if err != nil {
		t.Fatal(err)
	}
	trash, err := env.store.ListDeletedEmails(ctx, "bob", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 0 {
		t.Fatalf("alice's trash sync trashed bob's %q", e.Subject)
	}
}
