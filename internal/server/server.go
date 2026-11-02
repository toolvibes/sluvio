// Package server exposes Sluvio over HTTP: a JSON API and the embedded UI.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/toolvibes/sluvio/internal/app"
	"github.com/toolvibes/sluvio/internal/cfgfile"
	"github.com/toolvibes/sluvio/internal/diff"
	"github.com/toolvibes/sluvio/internal/haproxy"
)

// Version is set at build time.
var Version = "dev"

// Server is the HTTP handler.
type Server struct {
	App  *app.App
	Auth *Auth
	UI   fs.FS // contents of web/dist
	Log  *slog.Logger
	// Settings shown on the setup page.
	Settings map[string]string
	mux      *http.ServeMux
}

// Handler builds the routes.
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/login", s.login)
	m.HandleFunc("POST /api/logout", s.logout)
	m.HandleFunc("GET /api/me", s.me)

	m.HandleFunc("GET /api/config", s.authed(s.getConfig))
	m.HandleFunc("GET /api/config/raw", s.authed(s.getRaw))
	m.HandleFunc("GET /api/config/section", s.authed(s.getSection))
	m.HandleFunc("POST /api/changes/preview", s.authed(s.preview))
	m.HandleFunc("POST /api/changes/apply", s.authed(s.apply))
	m.HandleFunc("GET /api/status", s.authed(s.status))
	m.HandleFunc("POST /api/runtime/server", s.authed(s.runtimeServer))
	m.HandleFunc("GET /api/history", s.authed(s.historyList))
	m.HandleFunc("GET /api/history/{id}", s.authed(s.historyGet))
	m.HandleFunc("POST /api/history/{id}/rollback", s.authed(s.rollback))
	m.HandleFunc("GET /api/system", s.authed(s.system))
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown endpoint")
	})
	m.Handle("/", s.static())
	s.mux = m
	return s.middleware(m)
}

// middleware adds security headers, CSRF protection and logging.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		// State changing API calls must carry a custom header. Browsers do not
		// send custom headers cross-origin without a CORS preflight, which
		// this server never allows.
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-Sluvio") != "1" {
				writeErr(w, http.StatusForbidden, "csrf", "missing X-Sluvio header")
				return
			}
		}
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(rw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/status" && s.Log != nil {
			s.Log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rw.code, "duration", time.Since(start).Round(time.Millisecond))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }

func (s *Server) authed(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.Auth.User(r)
		if u == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "login required")
			return
		}
		h(w, r, u)
	}
}

// ---------------------------------------------------------------------------
// helpers

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Params  map[string]string `json:"params,omitempty"`
	// Validation is set when HAProxy rejected the config.
	Validation *haproxy.Validation `json:"validation,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, c, msg string) {
	writeJSON(w, code, map[string]any{"error": apiError{Code: c, Message: msg}})
}

// writeAppErr maps errors to HTTP responses.
func writeAppErr(w http.ResponseWriter, err error) {
	var ce *cfgfile.Error
	var ve *app.ValidationError
	switch {
	case errors.Is(err, app.ErrConflict):
		writeErr(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, app.ErrReadOnly):
		writeErr(w, http.StatusForbidden, "read_only", err.Error())
	case errors.Is(err, haproxy.ErrNoRuntime):
		writeErr(w, http.StatusServiceUnavailable, "no_runtime", err.Error())
	case errors.As(err, &ce):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": apiError{Code: ce.Code, Message: ce.Message, Params: ce.Params}})
	case errors.As(err, &ve):
		v := ve.Validation
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": apiError{Code: "invalid_config", Message: err.Error(), Validation: &v}})
	default:
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 8<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &cfgfile.Error{Code: "bad_request", Message: "invalid request body: " + err.Error()}
	}
	return nil
}

// ---------------------------------------------------------------------------
// auth

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeAppErr(w, err)
		return
	}
	if err := s.Auth.Login(w, r, req.Username, req.Password); err != nil {
		code := http.StatusUnauthorized
		c := "bad_login"
		if errors.Is(err, errTooMany) {
			code, c = http.StatusTooManyRequests, "too_many_attempts"
		}
		if s.Log != nil {
			s.Log.Warn("login failed", "user", req.Username, "ip", clientIP(r))
		}
		writeErr(w, code, c, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": req.Username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.Auth.Logout(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := s.Auth.User(r)
	if u == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "authDisabled": s.Auth.Disabled, "readOnly": s.App.ReadOnly, "version": Version})
}

// ---------------------------------------------------------------------------
// configuration

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request, _ string) {
	snap, err := s.App.Load()
	if err != nil {
		writeAppErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  snap.Version,
		"path":     snap.Path,
		"modTime":  snap.ModTime,
		"readOnly": s.App.ReadOnly,
		"model":    snap.Doc.Model(),
	})
}

func (s *Server) getRaw(w http.ResponseWriter, r *http.Request, _ string) {
	snap, err := s.App.Load()
	if err != nil {
		writeAppErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": snap.Version, "content": snap.Content, "path": snap.Path})
}

func (s *Server) getSection(w http.ResponseWriter, r *http.Request, _ string) {
	snap, err := s.App.Load()
	if err != nil {
		writeAppErr(w, err)
		return
	}
	text, err := snap.Doc.SectionText(r.URL.Query().Get("kind"), r.URL.Query().Get("name"))
	if err != nil {
		writeAppErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": snap.Version, "content": text})
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request, _ string) {
	var req app.ChangeRequest
	if err := decode(r, &req); err != nil {
		writeAppErr(w, err)
		return
	}
	p, err := s.App.Preview(r.Context(), req)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) apply(w http.ResponseWriter, r *http.Request, user string) {
	var req app.ChangeRequest
	if err := decode(r, &req); err != nil {
		writeAppErr(w, err)
		return
	}
	// Do not let a client disconnect abort a half-done reload.
	ctx := context.WithoutCancel(r.Context())
	res, err := s.App.Apply(ctx, user, req)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	if s.Log != nil && res.Changed {
		s.Log.Info("configuration changed", "user", user, "ops", len(req.Ops), "reloaded", res.Reloaded, "reloadError", res.ReloadError)
	}
	writeJSON(w, http.StatusOK, res)
}

// ---------------------------------------------------------------------------
// runtime

func (s *Server) status(w http.ResponseWriter, r *http.Request, _ string) {
	writeJSON(w, http.StatusOK, s.App.Status())
}

func (s *Server) runtimeServer(w http.ResponseWriter, r *http.Request, user string) {
	var req struct {
		Backend string `json:"backend"`
		Server  string `json:"server"`
		State   string `json:"state"`
	}
	if err := decode(r, &req); err != nil {
		writeAppErr(w, err)
		return
	}
	if err := s.App.SetServerState(req.Backend, req.Server, req.State); err != nil {
		if errors.Is(err, haproxy.ErrNoRuntime) || errors.Is(err, app.ErrReadOnly) {
			writeAppErr(w, err)
			return
		}
		writeErr(w, http.StatusBadRequest, "runtime_error", err.Error())
		return
	}
	if s.Log != nil {
		s.Log.Info("runtime state changed", "user", user, "server", req.Backend+"/"+req.Server, "state", req.State)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// history

func (s *Server) historyList(w http.ResponseWriter, r *http.Request, _ string) {
	if _, err := s.App.Load(); err != nil { // records external changes
		writeAppErr(w, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	l, err := s.App.History.List(limit)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": l})
}

func (s *Server) historyGet(w http.ResponseWriter, r *http.Request, _ string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	e, content, err := s.App.History.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	_, prev, _ := s.App.History.Previous(id)
	snap, _ := s.App.Load()
	resp := map[string]any{"entry": e, "content": content, "diff": diff.Compute(prev, content, 3)}
	if snap != nil {
		resp["diffToCurrent"] = diff.Compute(content, snap.Content, 3)
		resp["current"] = snap.Version
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) rollback(w http.ResponseWriter, r *http.Request, user string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req struct {
		Version string `json:"version"`
	}
	if err := decode(r, &req); err != nil {
		writeAppErr(w, err)
		return
	}
	res, err := s.App.Rollback(context.WithoutCancel(r.Context()), user, id, req.Version)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---------------------------------------------------------------------------
// setup checks

type check struct {
	ID     string            `json:"id"`
	OK     bool              `json:"ok"`
	Level  string            `json:"level"` // ok, warn, error
	Detail string            `json:"detail,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

func (s *Server) system(w http.ResponseWriter, r *http.Request, _ string) {
	var checks []check
	add := func(id string, ok bool, failLevel, detail string, params map[string]string) {
		lvl := "ok"
		if !ok {
			lvl = failLevel
		}
		checks = append(checks, check{ID: id, OK: ok, Level: lvl, Detail: detail, Params: params})
	}
	snap, err := s.App.Load()
	add("config_readable", err == nil, "error", errString(err), map[string]string{"path": s.App.ConfigPath})
	if err == nil {
		f, werr := os.OpenFile(s.App.ConfigPath, os.O_WRONLY, 0)
		if werr == nil {
			f.Close()
		}
		add("config_writable", werr == nil || s.App.ReadOnly, "error", errString(werr), map[string]string{"path": s.App.ConfigPath})
	}
	ver, verr := s.App.Validator.Version(r.Context())
	add("validator", verr == nil, "warn", firstNonEmpty(ver, errString(verr)), map[string]string{"binary": s.App.Validator.Binary})
	st := s.App.Status()
	runtimeDetail := st.Error
	if st.Info != nil {
		runtimeDetail = "HAProxy " + st.Info.Version + ", uptime " + st.Info.Uptime
	}
	add("runtime", st.Available, "warn", runtimeDetail, nil)
	add("reload", s.App.Runtime != nil && s.App.Runtime.CanReload(), "warn", "", nil)
	if verr == nil && st.Info != nil {
		bin := versionNumber(ver)
		run := versionNumber(st.Info.Version)
		add("version_match", majorMinor(bin) == majorMinor(run), "warn", "", map[string]string{"validator": bin, "running": run})
	}
	if snap != nil && !st.Available && !strings.Contains(snap.Content, "stats socket") {
		add("stats_socket_hint", false, "warn", "", nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  Version,
		"checks":   checks,
		"settings": s.Settings,
		"readOnly": s.App.ReadOnly,
	})
}

func versionNumber(v string) string {
	for _, f := range strings.Fields(v) {
		if len(f) > 0 && f[0] >= '0' && f[0] <= '9' && strings.Contains(f, ".") {
			return f
		}
	}
	return v
}

func majorMinor(v string) string {
	p := strings.SplitN(v, ".", 3)
	if len(p) >= 2 {
		return p[0] + "." + p[1]
	}
	return v
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// static UI

func (s *Server) static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.UI == nil {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if st, err := fs.Stat(s.UI, p); err != nil || st.IsDir() {
			p = "index.html" // SPA fallback
		}
		f, err := s.UI.Open(p)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "Sluvio API is running, but the web UI was not built.\nRun: cd web && npm ci && npm run build\n")
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, p, st.ModTime(), f.(io.ReadSeeker))
	})
}
