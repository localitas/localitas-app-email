package email

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"testing/fstest"

	client "github.com/localitas/localitas-go"
	_ "github.com/mattn/go-sqlite3"
)

const testAPIToken = "lt_test_email_cluster_token"

func TestMain(m *testing.M) {
	os.Setenv("LOCALITAS_API_TOKEN", testAPIToken)
	os.Setenv("LOCALITAS_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	client.SetIdentityKeyFromAPIToken(testAPIToken)
	os.Exit(m.Run())
}

// identityToken signs claims the way core does for a request it forwards to
// the app: base64url JSON, a dot, and the base64url HMAC-SHA256 of the payload
// under the key derived from the cluster API token.
func identityToken(userID, permission string) string {
	raw, _ := json.Marshal(client.TokenInfo{UserID: userID, Email: userID + "@example.com", Permission: permission})
	payload := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte("localitas-internal-token-v1:" + testAPIToken))
	mac := hmac.New(sha256.New, []byte(base64.StdEncoding.EncodeToString(sum[:])))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// openTestStore opens a fresh SQLite database migrated with the app's own
// embedded migrations.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+url.PathEscape(t.Name())+".sqlitedb?mode=memory&cache=shared&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(0)
	t.Cleanup(func() { db.Close() })

	files := fstest.MapFS{}
	entries, err := fs.ReadDir(migrationsFS, "_migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := fs.ReadFile(migrationsFS, "_migrations/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		files["migrations/"+e.Name()] = &fstest.MapFile{Data: data}
	}
	m, err := client.NewMigrator(db, files, "email")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

// closedPort returns a local port nothing listens on, so IMAP and SMTP
// connections to it fail at once.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// mailbox is one user's seeded account and the records under it.
type mailbox struct {
	user       string
	account    *Account
	folder     *Folder
	email      *Email
	threadID   string
	attachment *Attachment
	filter     *Filter
	draft      *Draft
}

type testEnv struct {
	t     *testing.T
	store *Store
	mux   *http.ServeMux
	alice *mailbox
	bob   *mailbox
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	store := openTestStore(t)
	app := New(nil, "/")
	app.Store = store
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	env := &testEnv{t: t, store: store, mux: mux}
	port := closedPort(t)
	env.alice = env.seed("alice", port, "")
	env.bob = env.seed("bob", port, "")
	return env
}

// seed gives user an account with a folder, an email (unsubscribe target
// unsubURL), an attachment, a filter and a draft.
func (env *testEnv) seed(user string, port int, unsubURL string) *mailbox {
	t := env.t
	t.Helper()
	ctx := context.Background()
	s := env.store
	acct, err := s.CreateAccount(ctx, user, user, user+"@example.com", "imap", "127.0.0.1", port, "127.0.0.1", port, user, user+"-password", "", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	folder, err := s.UpsertFolder(ctx, acct.ID, "INBOX", "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	mb := &mailbox{user: user, account: acct, folder: folder, threadID: "<thread-" + user + "@example.com>"}
	mb.email = env.addEmail(mb, 1, "quarterly "+user+" report", unsubURL)
	if mb.attachment, err = s.InsertAttachment(ctx, mb.email.ID, acct.ID, user+".pdf", "application/pdf", "email-attachments/"+mb.email.ID+"/"+user+".pdf", 10); err != nil {
		t.Fatal(err)
	}
	if mb.filter, err = s.CreateFilter(ctx, user, acct.ID, "from:newsletter => star"); err != nil {
		t.Fatal(err)
	}
	if mb.draft, err = s.SaveDraft(ctx, user, "", acct.ID, "friend@example.com", "", "", user+" draft", "body", "", ""); err != nil {
		t.Fatal(err)
	}
	return mb
}

// addEmail stores a message in mb's inbox and returns it.
func (env *testEnv) addEmail(mb *mailbox, uid uint32, subject, unsubURL string) *Email {
	t := env.t
	t.Helper()
	ctx := context.Background()
	msgID := "<msg-" + mb.user + "-" + strconv.Itoa(int(uid)) + "@example.com>"
	if err := env.store.InsertEmail(ctx, mb.account.ID, mb.folder.ID, msgID, subject, "Sender", "sender@example.com", mb.user+"@example.com", "snippet", subject+" body", "", 1700000000+int64(uid), uid, true, false, mb.threadID, "", unsubURL, false); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := env.store.db.QueryRowContext(ctx, "SELECT id FROM emails WHERE message_id = ?", msgID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	e, err := env.store.GetEmail(ctx, mb.user, id)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// do sends a request as user ("" sends none) with the given scope.
func (env *testEnv) do(method, path, user, scope string, body any) *httptest.ResponseRecorder {
	env.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			env.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+identityToken(user, scope))
	}
	w := httptest.NewRecorder()
	env.mux.ServeHTTP(w, req)
	return w
}

func (env *testEnv) as(user, method, path string, body any) *httptest.ResponseRecorder {
	return env.do(method, path, user, "write", body)
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int, what string) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("%s: status %d, want %d (body %s)", what, w.Code, want, w.Body.String())
	}
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	return v
}
