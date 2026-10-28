package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"kaogong/internal/auth"
	"kaogong/internal/config"
	"kaogong/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	hash, err := auth.HashPassword("admin123456")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('admin', ?, 'admin')`, hash); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{WebDir: "../../public", SessionTTL: time.Hour}
	s := New(cfg, db, auth.NewManager(db, cfg.SessionTTL))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func postJSON(t *testing.T, client *http.Client, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func testClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: ts.Client().Transport, Jar: jar}
}

func TestLoginAndProtectedRoute(t *testing.T) {
	ts, _ := newTestServer(t)
	client := testClient(t, ts)

	res, err := client.Get(ts.URL + "/api/subjects")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", res.StatusCode)
	}
	res.Body.Close()

	res = postJSON(t, client, ts.URL+"/api/auth/login", credentials{Username: "admin", Password: "admin123456"})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", res.StatusCode)
	}
	if len(res.Cookies()) == 0 {
		t.Fatal("login did not set a session cookie")
	}

	res, err = client.Get(ts.URL + "/api/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d", res.StatusCode)
	}
}

func TestNonAdminCannotUseAdminRoute(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := testClient(t, ts)
	res := postJSON(t, admin, ts.URL+"/api/auth/login", credentials{Username: "admin", Password: "admin123456"})
	res.Body.Close()

	res = postJSON(t, admin, ts.URL+"/api/admin/users", createUserRequest{Username: "alice", Password: "alice123456", Role: "user"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create user status = %d", res.StatusCode)
	}
	res.Body.Close()

	user := testClient(t, ts)
	res = postJSON(t, user, ts.URL+"/api/auth/login", credentials{Username: "alice", Password: "alice123456"})
	res.Body.Close()
	res, err := user.Get(ts.URL + "/api/admin/users")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", res.StatusCode)
	}
}
