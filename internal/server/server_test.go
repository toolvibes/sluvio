package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/toolvibes/sluiceops/internal/app"
	"github.com/toolvibes/sluiceops/internal/haproxy"
	"github.com/toolvibes/sluiceops/internal/history"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "haproxy.cfg")
	os.WriteFile(cfg, []byte("defaults\n    mode http\n\nbackend be\n    server s1 127.0.0.1:1\n"), 0o644)
	hist, _ := history.Open(filepath.Join(dir, "history"), 0)
	a := &app.App{ConfigPath: cfg, Validator: &haproxy.Validator{ConfigPath: cfg}, Runtime: &haproxy.Runtime{}, History: hist}
	auth, gen, err := NewAuth(dir, "admin", "", 0)
	if err != nil || gen == "" {
		t.Fatalf("expected generated password: %v", err)
	}
	s := &Server{App: a, Auth: auth, UI: fstest.MapFS{"index.html": {Data: []byte("<html>ui</html>")}}}
	return httptest.NewServer(s.Handler()), gen
}

func do(t *testing.T, c *http.Client, method, url, body string, csrf bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if csrf {
		req.Header.Set("X-SluiceOps", "1")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func TestAuthAndCSRF(t *testing.T) {
	ts, pw := newTestServer(t)
	defer ts.Close()
	c := ts.Client()
	if r := do(t, c, "GET", ts.URL+"/api/config", "", false); r.StatusCode != 401 {
		t.Fatalf("unauthenticated access: %d", r.StatusCode)
	}
	if r := do(t, c, "POST", ts.URL+"/api/login", `{"username":"admin","password":"`+pw+`"}`, false); r.StatusCode != 403 {
		t.Fatalf("login without CSRF header: %d", r.StatusCode)
	}
	if r := do(t, c, "POST", ts.URL+"/api/login", `{"username":"admin","password":"wrong-password"}`, true); r.StatusCode != 401 {
		t.Fatalf("wrong password: %d", r.StatusCode)
	}
	r := do(t, c, "POST", ts.URL+"/api/login", `{"username":"admin","password":"`+pw+`"}`, true)
	if r.StatusCode != 200 {
		t.Fatalf("login: %d", r.StatusCode)
	}
	var cookie *http.Cookie
	for _, ck := range r.Cookies() {
		if ck.Name == cookieName {
			cookie = ck
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("bad session cookie: %+v", cookie)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/config", nil)
	req.AddCookie(cookie)
	res, _ := c.Do(req)
	if res.StatusCode != 200 {
		t.Fatalf("authenticated access: %d", res.StatusCode)
	}
	res.Body.Close()
	// A mutating call with a valid session but without the header is refused.
	req, _ = http.NewRequest("POST", ts.URL+"/api/changes/apply", strings.NewReader(`{"ops":[]}`))
	req.AddCookie(cookie)
	res, _ = c.Do(req)
	if res.StatusCode != 403 {
		t.Fatalf("apply without CSRF header: %d", res.StatusCode)
	}
	res.Body.Close()
	// SPA fallback serves the UI for unknown paths.
	res, _ = c.Get(ts.URL + "/services/backend-api")
	if res.StatusCode != 200 || res.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("ui: %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestLoginRateLimit(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()
	c := ts.Client()
	for i := 0; i < 5; i++ {
		do(t, c, "POST", ts.URL+"/api/login", `{"username":"admin","password":"wrong-password"}`, true)
	}
	if r := do(t, c, "POST", ts.URL+"/api/login", `{"username":"admin","password":"wrong-password"}`, true); r.StatusCode != 429 {
		t.Fatalf("expected 429, got %d", r.StatusCode)
	}
}
