package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/toolvibes/sluvio/internal/cfgfile"
	"github.com/toolvibes/sluvio/internal/haproxy"
	"github.com/toolvibes/sluvio/internal/history"
)

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startHAProxy runs a master-worker HAProxy like the official Docker image does.
func startHAProxy(t *testing.T) (*App, string) {
	t.Helper()
	bin, err := exec.LookPath("haproxy")
	if err != nil {
		t.Skip("haproxy not installed")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "haproxy.cfg")
	sock := filepath.Join(dir, "master.sock")
	port := freePort(t)
	content := fmt.Sprintf(`global
    log stdout format raw local0 info

defaults
    mode http
    timeout connect 5s
    timeout client  30s
    timeout server  30s

frontend http
    bind 127.0.0.1:%d
    acl is_a_host hdr(host) -i a.example
    # acl is_b_host hdr(host) -i b.example
    use_backend be-a if is_a_host
    # use_backend be-b if is_b_host
    default_backend be-a

# Service A
backend be-a
    server a1 127.0.0.1:1 check
    server a2 127.0.0.1:2

# Service B (prepared, disabled)
# backend be-b
#     server b1 127.0.0.1:3
`, port)
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-W", "-db", "-f", cfg, "-S", sock)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// The socket file appears before HAProxy accepts connections on it.
	probe := &haproxy.Runtime{MasterSocket: sock, Timeout: time.Second}
	for i := 0; i < 50; i++ {
		if _, err := probe.ShowInfo(); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	hist, err := history.Open(filepath.Join(dir, "history"), 0)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{
		ConfigPath: cfg,
		Validator:  &haproxy.Validator{Binary: bin, ConfigPath: cfg},
		Runtime:    &haproxy.Runtime{MasterSocket: sock},
		History:    hist,
	}
	return a, cfg
}

func serverStatus(t *testing.T, a *App, backend, server string) string {
	t.Helper()
	for _, s := range a.Status().Stats {
		if s.Proxy == backend && s.Name == server {
			return s.Status
		}
	}
	return ""
}

func TestEndToEnd(t *testing.T) {
	a, cfg := startHAProxy(t)
	ctx := context.Background()
	st := a.Status()
	if !st.Available || st.Info == nil {
		t.Fatalf("runtime not available: %+v", st)
	}
	snap, err := a.Load()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Server maintenance is applied live, without reload.
	off := false
	res, err := a.Apply(ctx, "admin", ChangeRequest{Version: snap.Version, Ops: []cfgfile.Op{{Op: "server.setEnabled", Backend: "be-a", Name: "a2", Enabled: &off}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.ReloadRequired || res.Reloaded || len(res.RuntimeActions) != 1 || res.RuntimeActions[0].Error != "" {
		t.Fatalf("expected runtime activation: %+v", res)
	}
	if s := serverStatus(t, a, "be-a", "a2"); s != "MAINT" {
		t.Fatalf("a2 status %q, want MAINT", s)
	}
	b, _ := os.ReadFile(cfg)
	if !strings.Contains(string(b), "server a2 127.0.0.1:2 disabled") {
		t.Fatal("maintenance not persisted in file")
	}

	// 2. A stale version is rejected.
	if _, err := a.Apply(ctx, "admin", ChangeRequest{Version: snap.Version, Ops: []cfgfile.Op{{Op: "server.setEnabled", Backend: "be-a", Name: "a2", Enabled: &off}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// 3. Enabling the prepared service uncomments ACL, rule and backend and reloads.
	snap, _ = a.Load()
	on := true
	res, err = a.Apply(ctx, "admin", ChangeRequest{Version: snap.Version, Message: "enable B", Ops: []cfgfile.Op{{Op: "service.setEnabled", Backend: "be-b", Enabled: &on}}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reloaded {
		t.Fatalf("expected reload: %+v", res)
	}
	time.Sleep(300 * time.Millisecond)
	if s := serverStatus(t, a, "be-b", "b1"); s == "" {
		t.Fatal("be-b/b1 not present after reload")
	}
	// Maintenance of a2 survives the reload because it is in the file.
	if s := serverStatus(t, a, "be-a", "a2"); s != "MAINT" {
		t.Fatalf("a2 status after reload %q, want MAINT", s)
	}

	// 4. Invalid changes are not written.
	snap, _ = a.Load()
	bad := snap.Content + "\nbackend broken\n    server x 127.0.0.1:1 frobnicate\n"
	_, err = a.Apply(ctx, "admin", ChangeRequest{Version: snap.Version, Ops: []cfgfile.Op{{Op: "raw.replace", Content: &bad}}})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Validation.Valid || len(ve.Validation.Messages) == 0 {
		t.Fatalf("expected validation error, got %v", err)
	}
	if after, _ := os.ReadFile(cfg); string(after) != snap.Content {
		t.Fatal("invalid config was written")
	}
	found := false
	for _, m := range ve.Validation.Messages {
		if m.Line > 0 && strings.Contains(m.Message, "frobnicate") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a line reference to the error: %+v", ve.Validation.Messages)
	}

	// 5. History and rollback to the first version.
	list, _ := a.History.List(0)
	if len(list) < 3 {
		t.Fatalf("expected at least 3 versions, got %d", len(list))
	}
	first := list[len(list)-1]
	if first.Kind != "initial" {
		t.Fatalf("oldest entry should be initial, got %s", first.Kind)
	}
	snap, _ = a.Load()
	if _, err := a.Rollback(ctx, "admin", first.ID, snap.Version); err != nil {
		t.Fatal(err)
	}
	_, orig, _ := a.History.Get(first.ID)
	if now, _ := os.ReadFile(cfg); string(now) != orig {
		t.Fatal("rollback did not restore the original")
	}
	time.Sleep(300 * time.Millisecond)
	if s := serverStatus(t, a, "be-b", "b1"); s != "" {
		t.Fatal("be-b should be gone after rollback")
	}

	// 6. External edits are recorded.
	os.WriteFile(cfg, []byte(orig+"# hand edit\n"), 0o644)
	a.Load()
	latest, _ := a.History.Latest()
	if latest.Kind != "external" {
		t.Fatalf("external edit not recorded: %+v", latest)
	}
}
