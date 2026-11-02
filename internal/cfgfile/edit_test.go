package cfgfile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toolvibes/sluvio/internal/diff"
)

func ptr[T any](v T) *T { return &v }

// haproxyCheck validates a config with the real binary if it is installed.
func haproxyCheck(t *testing.T, content string) {
	t.Helper()
	bin, err := exec.LookPath("haproxy")
	if err != nil {
		return
	}
	p := filepath.Join(t.TempDir(), "haproxy.cfg")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "-c", "-f", p).CombinedOutput()
	if err != nil {
		t.Fatalf("haproxy -c failed: %v\n%s\n--- config ---\n%s", err, out, content)
	}
}

func apply(t *testing.T, d *Document, ops ...Op) *Document {
	t.Helper()
	nd, err := d.Apply(ops)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	haproxyCheck(t, nd.String())
	return nd
}

// changed returns the added and removed lines.
func changed(a, b *Document) (added, removed []string) {
	for _, l := range diff.Lines(a.String(), b.String()) {
		switch l.Op {
		case diff.Insert:
			added = append(added, l.Text)
		case diff.Delete:
			removed = append(removed, l.Text)
		}
	}
	return
}

func findRule(t *testing.T, d *Document, frontend, backend string) Rule {
	t.Helper()
	for _, f := range d.Model().Frontends {
		if f.Name != frontend {
			continue
		}
		for _, r := range f.Rules {
			if r.Backend == backend {
				return r
			}
		}
	}
	t.Fatalf("rule to %s not found", backend)
	return Rule{}
}

func findBackend(d *Document, name string) *Backend {
	for _, b := range d.Model().Backends {
		if b.Name == name {
			return b
		}
	}
	return nil
}

func TestFixtureIsValid(t *testing.T) {
	haproxyCheck(t, loadFixture(t, "realworld.cfg").String())
}

func TestProseIsNotConfiguration(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	for i, l := range d.Lines {
		if d.Info(i).Kind != KindDisabled {
			continue
		}
		t := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "#"))
		for _, prose := range []string{"Muss", "Vorerst", "Backend:", "Host-Routing", "Inject", "Signaling", "Readiness", "UI-Schutz", "node-"} {
			if strings.HasPrefix(t, prose) {
				panic("prose classified as configuration: " + l)
			}
		}
	}
}

func TestEnableDisabledService(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	if findBackend(d, "backend-recipes-api").Enabled {
		t.Fatal("recipes-api should start disabled")
	}
	nd := apply(t, d, Op{Op: "service.setEnabled", Backend: "backend-recipes-api", Enabled: ptr(true)})
	b := findBackend(nd, "backend-recipes-api")
	if !b.Enabled || len(b.Servers) != 2 {
		t.Fatalf("backend not enabled: %+v", b)
	}
	r := findRule(t, nd, "http", "backend-recipes-api")
	if !r.Enabled || len(r.Hosts) != 1 || r.Hosts[0] != "api.recipes.example.net" {
		t.Fatalf("rule not enabled: %+v", r)
	}
	// The sibling service stays disabled.
	if findRule(t, nd, "http", "backend-recipes-web").Enabled || findBackend(nd, "backend-recipes-web").Enabled {
		t.Fatal("recipes-web must stay disabled")
	}
	added, removed := changed(d, nd)
	// 1 acl + 1 use_backend + 1 header + 3 settings + 1 timeout + 2 servers = 9 lines
	if len(added) != 9 || len(removed) != 9 {
		t.Fatalf("expected 9 changed lines, got +%d -%d\n%v\n%v", len(added), len(removed), added, removed)
	}
	for _, l := range added {
		if strings.Contains(l, "#") {
			t.Errorf("line still commented: %q", l)
		}
	}
	// Disable again restores the original file byte for byte.
	back := apply(t, nd, Op{Op: "service.setEnabled", Backend: "backend-recipes-api", Enabled: ptr(false)})
	a2, r2 := changed(d, back)
	// The ACL stays enabled (unused ACLs are harmless), everything else is identical.
	if len(a2) != 1 || len(r2) != 1 || !strings.Contains(a2[0], "acl is_rc_api_host") {
		t.Fatalf("unexpected diff after disable: +%v -%v", a2, r2)
	}
}

func TestDisableActiveServiceRoundTrip(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	nd := apply(t, d, Op{Op: "service.setEnabled", Backend: "backend-bill-web", Enabled: ptr(false)})
	if findBackend(nd, "backend-bill-web").Enabled || findRule(t, nd, "http", "backend-bill-web").Enabled {
		t.Fatal("bill-web should be disabled")
	}
	back := apply(t, nd, Op{Op: "service.setEnabled", Backend: "backend-bill-web", Enabled: ptr(true)})
	if back.String() != d.String() {
		a, r := changed(d, back)
		t.Fatalf("round trip not identical: +%v -%v", a, r)
	}
}

func TestDefaultBackendCannotBeDisabled(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	_, err := d.Apply([]Op{{Op: "service.setEnabled", Backend: "backend-frontend", Enabled: ptr(false)}})
	if e, ok := err.(*Error); !ok || e.Code != "is_default" {
		t.Fatalf("expected is_default error, got %v", err)
	}
}

func TestEnableSingleCommentedRule(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	r := findRule(t, d, "http", "backend-calendar-api")
	nd := apply(t, d, Op{Op: "route.setEnabled", Frontend: "http", Line: ptr(r.Line), Enabled: ptr(true)})
	if !findBackend(nd, "backend-calendar-api").Enabled {
		t.Fatal("calendar-api backend should be enabled with its route")
	}
	if findBackend(nd, "backend-calendar-admin").Enabled {
		t.Fatal("calendar-admin must stay disabled")
	}
}

func TestServerMaintenanceRoundTrip(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	nd := apply(t, d, Op{Op: "server.setEnabled", Backend: "backend-api", Name: "api-2", Enabled: ptr(false)})
	added, _ := changed(d, nd)
	if len(added) != 1 || added[0] != "    server api-2 10.10.0.185:3000 check inter 5s fall 3 rise 2 disabled" {
		t.Fatalf("unexpected: %v", added)
	}
	back := apply(t, nd, Op{Op: "server.setEnabled", Backend: "backend-api", Name: "api-2", Enabled: ptr(true)})
	if back.String() != d.String() {
		t.Fatal("server round trip not identical")
	}
}

func TestTrailingCommentIsKept(t *testing.T) {
	d := Parse("backend b\n    server s1 1.2.3.4:80 check   # primary\n")
	nd, err := d.Apply([]Op{{Op: "server.setEnabled", Backend: "b", Name: "s1", Enabled: ptr(false)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := nd.Lines[1]; got != "    server s1 1.2.3.4:80 check disabled   # primary" {
		t.Fatalf("got %q", got)
	}
}

func TestCreateServiceInRealConfig(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	nd := apply(t, d,
		Op{Op: "backend.create", BackendSpec: &BackendSpec{
			Name: "backend-shop-api", Description: "Shop API (node-1:3060)", Balance: "leastconn",
			HealthCheck: &HealthCheck{Method: "GET", Path: "/healthz", Expect: "status 200"},
			Servers:     []ServerSpec{{Name: "shop-1", Address: "10.10.0.184", Port: "3060", Check: true, Inter: "5s", Fall: "3", Rise: "2"}},
		}},
		Op{Op: "route.create", Route: &RouteSpec{Frontend: "http", Backend: "backend-shop-api", Hosts: []string{"Shop.Example.io"}}},
	)
	r := findRule(t, nd, "http", "backend-shop-api")
	if r.Kind != "host" || r.Hosts[0] != "shop.example.io" || !r.Enabled {
		t.Fatalf("bad rule %+v", r)
	}
	// Host rules come before the path rules: the new rule must precede /api/.
	api := findRule(t, nd, "http", "backend-api")
	if r.Line > api.Line {
		t.Fatalf("host rule inserted after path rule (line %d > %d)", r.Line+1, api.Line+1)
	}
	// The ACL goes next to the other ACLs, not at the end of the frontend.
	var acl ACL
	for _, f := range nd.Model().Frontends {
		for _, a := range f.ACLs {
			if a.Name == "is_shop_api_host" {
				acl = a
			}
		}
	}
	if acl.Name == "" || acl.Line > r.Line {
		t.Fatalf("acl missing or misplaced: %+v", acl)
	}
	// New backend is appended at the very end (after the disabled calendar blocks).
	b := findBackend(nd, "backend-shop-api")
	if b == nil || b.Description != "Shop API (node-1:3060)" || b.Balance != "leastconn" {
		t.Fatalf("bad backend %+v", b)
	}
	added, removed := changed(d, nd)
	if len(removed) != 0 {
		t.Fatalf("nothing should be removed, got %v", removed)
	}
	if len(added) != 10 {
		t.Fatalf("expected 10 added lines, got %d: %q", len(added), added)
	}
}

func TestPathRouteGoesBeforeExistingPathRules(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	nd := apply(t, d, Op{Op: "route.create", Route: &RouteSpec{Frontend: "http", Backend: "backend-landing", Paths: []string{"/api/v2/"}}})
	r := findRule(t, nd, "http", "backend-landing")
	api := findRule(t, nd, "http", "backend-api")
	pt := findRule(t, nd, "http", "backend-pricetracker-api")
	if !(r.Line < api.Line && r.Line > pt.Line) {
		t.Fatalf("path rule at line %d, want between %d and %d", r.Line+1, pt.Line+1, api.Line+1)
	}
}

func TestRouteUpdateAndDelete(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	r := findRule(t, d, "http", "backend-bill-landing")
	nd := apply(t, d, Op{Op: "route.update", Frontend: "http", Line: ptr(r.Line), Route: &RouteSpec{
		Frontend: "http", Backend: "backend-bill-landing", Hosts: []string{"billing.example.de", "www.billing.example.de", "billing.example.com"},
	}})
	r2 := findRule(t, nd, "http", "backend-bill-landing")
	if len(r2.Hosts) != 3 {
		t.Fatalf("hosts not updated: %+v", r2)
	}
	added, removed := changed(d, nd)
	// Minimal diff: only the new host line is added, alignment of the
	// existing lines is untouched.
	if len(removed) != 0 || len(added) != 1 || added[0] != "    acl is_bill_landing_host hdr(host) -i billing.example.com" {
		t.Fatalf("unexpected diff +%q -%q", added, removed)
	}
	// Existing ACL name is kept.
	if r2.CondTest != "is_bill_landing_host" {
		t.Fatalf("condition changed: %q", r2.CondTest)
	}
	nd2 := apply(t, nd, Op{Op: "route.delete", Frontend: "http", Line: ptr(r2.Line)})
	for _, f := range nd2.Model().Frontends {
		for _, a := range f.ACLs {
			if a.Name == "is_bill_landing_host" {
				t.Fatal("exclusive ACL should be removed with the route")
			}
		}
	}
	// ACLs shared with other lines stay: is_ws is also used by http-request.
	r3 := findRule(t, nd2, "http", "backend-signal")
	nd3 := apply(t, nd2, Op{Op: "route.update", Frontend: "http", Line: ptr(r3.Line), Route: &RouteSpec{
		Frontend: "http", Backend: "backend-signal", Hosts: []string{"stream.example.net"}, Paths: []string{"/rtc"},
	}})
	r4 := findRule(t, nd3, "http", "backend-signal")
	if r4.Kind != "host+path" || r4.CondTest != "is_signal_host is_signal_path" {
		t.Fatalf("host+path update wrong: %+v", r4)
	}
}

func TestBackendUpdate(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	nd := apply(t, d, Op{Op: "backend.update", Name: "backend-frontend", BackendSpec: &BackendSpec{
		Balance:     "leastconn",
		HealthCheck: &HealthCheck{Method: "GET", Path: "/health", Expect: "status 200"},
		Timeouts:    map[string]string{"server": "120s"},
	}})
	b := findBackend(nd, "backend-frontend")
	if b.Balance != "leastconn" || b.HealthCheck.Path != "/health" || b.Timeouts["server"] != "120s" {
		t.Fatalf("not updated: %+v", b)
	}
	added, removed := changed(d, nd)
	if len(added) != 3 || len(removed) != 2 {
		t.Fatalf("unexpected diff +%q -%q", added, removed)
	}
	// Removing the health check removes both lines.
	nd2 := apply(t, nd, Op{Op: "backend.update", Name: "backend-frontend", BackendSpec: &BackendSpec{Balance: "leastconn", Timeouts: map[string]string{"server": "120s"}}})
	if findBackend(nd2, "backend-frontend").HealthCheck != nil {
		t.Fatal("health check should be removed")
	}
}

func TestBackendDeleteInUse(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	if _, err := d.Apply([]Op{{Op: "backend.delete", Name: "backend-api"}}); err == nil {
		t.Fatal("expected in_use error")
	}
	// A disabled backend can be deleted including its banner comment.
	nd := apply(t, d, Op{Op: "backend.delete", Name: "backend-recipes-web"})
	if findBackend(nd, "backend-recipes-web") != nil {
		t.Fatal("not deleted")
	}
	if strings.Contains(nd.String(), "Recipes Web (SSR") {
		t.Fatal("banner comment should be removed together with the backend")
	}
	if !strings.Contains(nd.String(), "Recipes API (port 3000)") {
		t.Fatal("neighbouring banner must stay")
	}
}

func TestUncommentBlockInsideSectionIsMoved(t *testing.T) {
	src := "frontend fe\n    bind :80\n# backend old\n#     server s 127.0.0.1:80\n    default_backend be\n\nbackend be\n    server x 127.0.0.1:81\n"
	d := Parse(src)
	nd, err := d.Apply([]Op{{Op: "service.setEnabled", Backend: "old", Enabled: ptr(true)}})
	if err != nil {
		t.Fatal(err)
	}
	f := nd.Model().Frontends[0]
	if f.DefaultBackend != "be" {
		t.Fatal("frontend lost its default_backend")
	}
	if b := findBackend(nd, "old"); b == nil || !b.Enabled || len(b.Servers) != 1 {
		t.Fatalf("backend not enabled correctly: %+v\n%s", b, nd)
	}
	haproxyCheck(t, nd.String())
}

func TestNestedCommentRoundTrip(t *testing.T) {
	src := "frontend fe\n    bind :80\n    # acl old hdr(host) -i old.example\n    # Erklärung zum Routing\n    default_backend be\n\nbackend be\n    server x 127.0.0.1:81\n"
	d := Parse(src)
	// Commenting a whole section nests existing comments one level deeper …
	d.commentSection(d.FindSection("frontend", "fe"))
	if d.FindBlock("frontend", "fe") == nil {
		t.Fatalf("frontend block not detected:\n%s", d)
	}
	// … so that enabling it restores the exact original.
	d.uncommentBlock(d.FindBlock("frontend", "fe"))
	if d.String() != src {
		t.Fatalf("got:\n%s\nwant:\n%s", d, src)
	}
}

func TestCreateFrontendAndDefault(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	nd := apply(t, d,
		Op{Op: "frontend.create", FrontendSpec: &FrontendSpec{Name: "internal", Binds: []string{"*:8090"}, DefaultBackend: "backend-api"}},
		Op{Op: "frontend.setDefaultBackend", Frontend: "landing", Backend: "backend-frontend"},
	)
	var found bool
	for _, f := range nd.Model().Frontends {
		if f.Name == "internal" && f.DefaultBackend == "backend-api" {
			found = true
		}
		if f.Name == "landing" && f.DefaultBackend != "backend-frontend" {
			t.Fatal("default backend not changed")
		}
	}
	if !found {
		t.Fatal("frontend not created")
	}
}

func TestInvalidInputIsRejected(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	cases := []Op{
		{Op: "route.create", Route: &RouteSpec{Frontend: "http", Backend: "x", Hosts: []string{"bad host"}}},
		{Op: "route.create", Route: &RouteSpec{Frontend: "http", Backend: "x", Paths: []string{"nopath"}}},
		{Op: "route.create", Route: &RouteSpec{Frontend: "http", Backend: "x"}},
		{Op: "backend.create", BackendSpec: &BackendSpec{Name: "has space"}},
		{Op: "backend.create", BackendSpec: &BackendSpec{Name: "backend-api"}},
		{Op: "server.add", Backend: "backend-api", Server: &ServerSpec{Name: "api-1", Address: "1.2.3.4"}},
		{Op: "server.add", Backend: "backend-api", Server: &ServerSpec{Name: "x", Address: "1.2.3.4 # inject"}},
	}
	for k, op := range cases {
		if _, err := d.Apply([]Op{op}); err == nil {
			t.Errorf("case %d (%s) should fail", k, op.Op)
		}
	}
}
