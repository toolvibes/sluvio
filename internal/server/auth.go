package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// dummyHash is compared against for unknown users so that a login attempt
// always costs one bcrypt comparison.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("sluiceops-dummy"), bcrypt.DefaultCost)

// Auth manages users and sessions.
type Auth struct {
	Disabled bool // --auth=none
	TTL      time.Duration
	file     string
	mu       sync.Mutex
	users    map[string]string // name -> bcrypt hash
	sessions map[string]*session
	failures map[string][]time.Time
}

type session struct {
	User    string
	Expires time.Time
}

const cookieName = "sluiceops_session"

// NewAuth loads the user file from dataDir. If adminPassword is set, the admin
// user is created or its password replaced. If no user exists, a random
// password is generated and returned so it can be logged once.
func NewAuth(dataDir, adminUser, adminPassword string, ttl time.Duration) (*Auth, string, error) {
	a := &Auth{TTL: ttl, file: filepath.Join(dataDir, "users.json"), users: map[string]string{},
		sessions: map[string]*session{}, failures: map[string][]time.Time{}}
	if a.TTL == 0 {
		a.TTL = 12 * time.Hour
	}
	if b, err := os.ReadFile(a.file); err == nil {
		if err := json.Unmarshal(b, &a.users); err != nil {
			return nil, "", fmt.Errorf("cannot read %s: %w", a.file, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	generated := ""
	if adminPassword == "" && len(a.users) == 0 {
		generated = randomToken(12)
		adminPassword = generated
	}
	if adminPassword != "" {
		if err := a.SetPassword(adminUser, adminPassword); err != nil {
			return nil, "", err
		}
	}
	return a, generated, nil
}

// SetPassword creates or updates a user.
func (a *Auth) SetPassword(user, password string) error {
	if len(password) < 8 {
		return errors.New("password must have at least 8 characters")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.users[user] = string(h)
	b, _ := json.MarshalIndent(a.users, "", "  ")
	return os.WriteFile(a.file, b, 0o600)
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tooManyFailures implements a simple per-IP limit: 5 failed logins per 5 minutes.
func (a *Auth) tooManyFailures(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cut := time.Now().Add(-5 * time.Minute)
	var keep []time.Time
	for _, t := range a.failures[ip] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	a.failures[ip] = keep
	return len(keep) >= 5
}

func (a *Auth) recordFailure(ip string) {
	a.mu.Lock()
	a.failures[ip] = append(a.failures[ip], time.Now())
	a.mu.Unlock()
}

// Login checks credentials and creates a session.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request, user, password string) error {
	ip := clientIP(r)
	if a.tooManyFailures(ip) {
		return errTooMany
	}
	a.mu.Lock()
	hash, ok := a.users[user]
	a.mu.Unlock()
	if !ok {
		// constant work to avoid user enumeration through timing
		hash = string(dummyHash)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || !ok {
		a.recordFailure(ip)
		return errBadLogin
	}
	token := randomToken(32)
	a.mu.Lock()
	a.sessions[token] = &session{User: user, Expires: time.Now().Add(a.TTL)}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: isHTTPS(r), MaxAge: int(a.TTL.Seconds()),
	})
	return nil
}

var (
	errBadLogin = errors.New("invalid username or password")
	errTooMany  = errors.New("too many failed logins, try again in a few minutes")
)

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// Logout ends the session.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// User returns the logged-in user or "".
func (a *Auth) User(r *http.Request) string {
	if a.Disabled {
		return "anonymous"
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, s := range a.sessions {
		if time.Now().After(s.Expires) {
			delete(a.sessions, k)
		}
	}
	for k, s := range a.sessions {
		if subtle.ConstantTimeCompare([]byte(k), []byte(c.Value)) == 1 {
			s.Expires = time.Now().Add(a.TTL) // sliding expiry
			return s.User
		}
	}
	return ""
}
