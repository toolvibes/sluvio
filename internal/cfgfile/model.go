package cfgfile

import (
	"sort"
	"strings"
)

// Model is a read-only, structured view of a document.
type Model struct {
	Frontends []*Frontend `json:"frontends"`
	Backends  []*Backend  `json:"backends"`
	Other     []*Generic  `json:"other"` // global, defaults, userlist, …
	Indent    string      `json:"-"`
}

// Generic describes a section the UI does not model in detail.
type Generic struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Line        int    `json:"line"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
}

// Frontend is a frontend or listen section.
type Frontend struct {
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	Line           int    `json:"line"`
	Enabled        bool   `json:"enabled"`
	Description    string `json:"description,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Binds          []Bind `json:"binds"`
	ACLs           []ACL  `json:"acls"`
	Rules          []Rule `json:"rules"`
	DefaultBackend string `json:"defaultBackend,omitempty"`
	DefaultLine    int    `json:"defaultLine,omitempty"`
}

// Bind is a "bind" line.
type Bind struct {
	Line    int      `json:"line"`
	Address string   `json:"address"`
	Params  []string `json:"params,omitempty"`
	SSL     bool     `json:"ssl"`
	Enabled bool     `json:"enabled"`
}

// ACL is an "acl" line.
type ACL struct {
	Line      int      `json:"line"`
	Name      string   `json:"name"`
	Criterion string   `json:"criterion"`
	Flags     []string `json:"flags,omitempty"`
	Values    []string `json:"values"`
	Enabled   bool     `json:"enabled"`
	Kind      string   `json:"kind"` // host, path, src, other
}

// Rule is a "use_backend" line.
type Rule struct {
	Line     int      `json:"line"`
	Backend  string   `json:"backend"`
	Cond     string   `json:"cond,omitempty"` // if / unless
	CondTest string   `json:"condTest,omitempty"`
	Enabled  bool     `json:"enabled"`
	Kind     string   `json:"kind"` // host, path, host+path, other
	Hosts    []string `json:"hosts,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	Text     string   `json:"text"`
	Dynamic  bool     `json:"dynamic,omitempty"` // backend name contains %[…]
}

// Backend is a backend or listen section.
type Backend struct {
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Line        int               `json:"line"`
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description,omitempty"`
	Mode        string            `json:"mode,omitempty"`
	Balance     string            `json:"balance,omitempty"`
	HealthCheck *HealthCheck      `json:"healthCheck,omitempty"`
	Timeouts    map[string]string `json:"timeouts,omitempty"`
	Servers     []Server          `json:"servers"`
	UsedBy      []Usage           `json:"usedBy"`
}

// HealthCheck is "option httpchk" plus "http-check expect".
type HealthCheck struct {
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	Expect string `json:"expect,omitempty"` // e.g. "status 200"
}

// Usage records where a backend is referenced.
type Usage struct {
	Frontend string `json:"frontend"`
	Line     int    `json:"line"`
	Default  bool   `json:"default"`
	Enabled  bool   `json:"enabled"`
}

// Server is a "server" line.
type Server struct {
	Line        int      `json:"line"`
	Name        string   `json:"name"`
	Address     string   `json:"address"`
	Port        string   `json:"port,omitempty"`
	Enabled     bool     `json:"enabled"`     // not commented out
	Maintenance bool     `json:"maintenance"` // has the "disabled" keyword
	Check       bool     `json:"check"`
	Inter       string   `json:"inter,omitempty"`
	Fall        string   `json:"fall,omitempty"`
	Rise        string   `json:"rise,omitempty"`
	Weight      string   `json:"weight,omitempty"`
	Backup      bool     `json:"backup"`
	SSL         bool     `json:"ssl"`
	Verify      string   `json:"verify,omitempty"`
	Extra       []string `json:"extra,omitempty"` // unknown parameters, kept verbatim
	Text        string   `json:"text"`
}

// lineWords returns the words for an active or commented-out line.
func (d *Document) lineWords(i int) []Word { return d.info[i].Words }

// Model builds the structured view.
func (d *Document) Model() *Model {
	m := &Model{Indent: d.Indent()}
	all := make([]*Section, 0, len(d.sections)+len(d.blocks))
	all = append(all, d.sections...)
	all = append(all, d.blocks...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Header < all[j].Header })
	for _, s := range all {
		enabled := !s.Disabled
		if s.Disabled && (d.FindSection(s.Kind, s.Name) != nil) {
			continue // an active section with the same name wins
		}
		switch s.Kind {
		case "frontend", "listen":
			m.Frontends = append(m.Frontends, d.frontend(s, enabled))
			if s.Kind == "listen" {
				m.Backends = append(m.Backends, d.backend(s, enabled))
			}
		case "backend":
			m.Backends = append(m.Backends, d.backend(s, enabled))
		default:
			if s.Disabled {
				continue // commented-out examples such as a sample userlist
			}
			m.Other = append(m.Other, &Generic{Kind: s.Kind, Name: s.Name, Line: s.Header, Enabled: enabled, Description: d.Description(s)})
		}
	}
	// Usages.
	byName := map[string]*Backend{}
	for _, b := range m.Backends {
		if _, ok := byName[b.Name]; !ok {
			byName[b.Name] = b
		}
	}
	for _, f := range m.Frontends {
		for _, r := range f.Rules {
			if b := byName[r.Backend]; b != nil {
				b.UsedBy = append(b.UsedBy, Usage{Frontend: f.Name, Line: r.Line, Enabled: r.Enabled && f.Enabled})
			}
		}
		if f.DefaultBackend != "" {
			if b := byName[f.DefaultBackend]; b != nil {
				b.UsedBy = append(b.UsedBy, Usage{Frontend: f.Name, Line: f.DefaultLine, Default: true, Enabled: f.Enabled})
			}
		}
	}
	for _, b := range m.Backends {
		if b.UsedBy == nil {
			b.UsedBy = []Usage{}
		}
	}
	return m
}

func (d *Document) frontend(s *Section, enabled bool) *Frontend {
	f := &Frontend{Kind: s.Kind, Name: s.Name, Line: s.Header, Enabled: enabled, Description: d.Description(s),
		Binds: []Bind{}, ACLs: []ACL{}, Rules: []Rule{}}
	for _, i := range d.BodyLines(s) {
		li := d.info[i]
		if li.Kind != KindDirective && li.Kind != KindDisabled {
			continue
		}
		// Inside a disabled block, lines at the block's own depth are its
		// content; deeper ones were disabled individually before.
		lineEnabled := li.Kind == KindDirective || (s.Disabled && li.Depth == d.info[s.Header].Depth)
		w := li.Words
		switch w[0].Text {
		case "mode":
			if len(w) > 1 && lineEnabled {
				f.Mode = w[1].Text
			}
		case "bind":
			if len(w) > 1 {
				b := Bind{Line: i, Address: w[1].Text, Enabled: lineEnabled}
				for _, p := range w[2:] {
					b.Params = append(b.Params, p.Text)
					if p.Text == "ssl" {
						b.SSL = true
					}
				}
				f.Binds = append(f.Binds, b)
			}
		case "acl":
			if a, ok := parseACL(w); ok {
				a.Line = i
				a.Enabled = lineEnabled
				f.ACLs = append(f.ACLs, a)
			}
		case "use_backend":
			if len(w) >= 2 {
				r := Rule{Line: i, Backend: w[1].Text, Enabled: lineEnabled, Text: d.directiveText(i)}
				if len(w) >= 3 {
					r.Cond = w[2].Text
					r.CondTest = joinRaw(w[3:])
				}
				r.Dynamic = strings.Contains(r.Backend, "%[")
				f.Rules = append(f.Rules, r)
			}
		case "default_backend":
			if len(w) >= 2 && lineEnabled {
				f.DefaultBackend = w[1].Text
				f.DefaultLine = i
			}
		}
	}
	for k := range f.Rules {
		classifyRule(&f.Rules[k], f.ACLs)
	}
	return f
}

func joinRaw(w []Word) string {
	parts := make([]string, len(w))
	for i, x := range w {
		parts[i] = x.Raw
	}
	return strings.Join(parts, " ")
}

// aclArgFlags are ACL flags that take an argument.
var aclArgFlags = map[string]bool{"-m": true, "-f": true, "-u": true, "-M": true}

func parseACL(w []Word) (ACL, bool) {
	if len(w) < 3 {
		return ACL{}, false
	}
	a := ACL{Name: w[1].Text, Criterion: w[2].Text, Values: []string{}}
	rest := w[3:]
	flagsDone := false
	for k := 0; k < len(rest); k++ {
		t := rest[k].Text
		if !flagsDone && strings.HasPrefix(t, "-") && len(t) > 1 {
			if t == "--" {
				flagsDone = true
				continue
			}
			a.Flags = append(a.Flags, t)
			if aclArgFlags[t] && k+1 < len(rest) {
				a.Flags = append(a.Flags, rest[k+1].Text)
				k++
			}
			continue
		}
		flagsDone = true
		a.Values = append(a.Values, t)
	}
	a.Kind = criterionKind(a.Criterion)
	return a, true
}

// criterionKind classifies an ACL fetch.
func criterionKind(c string) string {
	lc := strings.ToLower(c)
	switch {
	case strings.Contains(lc, "(host"):
		return "host"
	case strings.HasPrefix(lc, "path") || strings.HasPrefix(lc, "req.path") ||
		strings.HasPrefix(lc, "url") || strings.HasPrefix(lc, "base"):
		return "path"
	case strings.HasPrefix(lc, "src"):
		return "src"
	}
	return "other"
}

// classifyRule determines what a use_backend rule matches on by looking at
// the ACLs it references (including inline { … } ACLs).
func classifyRule(r *Rule, acls []ACL) {
	r.Kind = "other"
	if r.Cond != "if" || r.CondTest == "" {
		if r.Cond == "" {
			r.Kind = "always"
		}
		return
	}
	words, _ := splitLine(r.CondTest)
	hasHost, hasPath, hasOther := false, false, false
	var hosts, paths []string
	for k := 0; k < len(words); k++ {
		t := words[k].Text
		switch {
		case t == "||" || t == "or":
			hasOther = true // alternatives make the rule hard to summarise
		case t == "{":
			// inline ACL until "}"
			j := k + 1
			for j < len(words) && words[j].Text != "}" {
				j++
			}
			if j-k-1 >= 1 {
				inner := append([]Word{{Text: "acl"}, {Text: "_inline"}}, words[k+1:j]...)
				if a, ok := parseACL(inner); ok {
					switch a.Kind {
					case "host":
						hasHost = true
						hosts = append(hosts, a.Values...)
					case "path":
						hasPath = true
						paths = append(paths, a.Values...)
					default:
						hasOther = true
					}
				}
			}
			k = j
		case strings.HasPrefix(t, "!"):
			hasOther = true
		default:
			found := false
			for _, a := range acls {
				if a.Name != t {
					continue
				}
				found = true
				switch a.Kind {
				case "host":
					hasHost = true
					hosts = append(hosts, a.Values...)
				case "path":
					hasPath = true
					paths = append(paths, a.Values...)
				default:
					hasOther = true
				}
			}
			if !found {
				hasOther = true
			}
		}
	}
	r.Hosts = dedupe(hosts)
	r.Paths = dedupe(paths)
	switch {
	case hasOther:
		r.Kind = "other"
	case hasHost && hasPath:
		r.Kind = "host+path"
	case hasHost:
		r.Kind = "host"
	case hasPath:
		r.Kind = "path"
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func (d *Document) backend(s *Section, enabled bool) *Backend {
	b := &Backend{Kind: s.Kind, Name: s.Name, Line: s.Header, Enabled: enabled, Description: d.Description(s), Servers: []Server{}}
	for _, i := range d.BodyLines(s) {
		li := d.info[i]
		if li.Kind != KindDirective && li.Kind != KindDisabled {
			continue
		}
		lineEnabled := li.Kind == KindDirective || (s.Disabled && li.Depth == d.info[s.Header].Depth)
		w := li.Words
		switch w[0].Text {
		case "mode":
			if len(w) > 1 && lineEnabled {
				b.Mode = w[1].Text
			}
		case "balance":
			if len(w) > 1 && lineEnabled {
				b.Balance = joinText(w[1:])
			}
		case "option":
			if len(w) >= 2 && w[1].Text == "httpchk" && lineEnabled {
				if b.HealthCheck == nil {
					b.HealthCheck = &HealthCheck{}
				}
				args := w[2:]
				switch len(args) {
				case 0:
				case 1:
					b.HealthCheck.Path = args[0].Text
				default:
					b.HealthCheck.Method = args[0].Text
					b.HealthCheck.Path = args[1].Text
				}
			}
		case "http-check":
			if len(w) >= 3 && w[1].Text == "expect" && lineEnabled {
				if b.HealthCheck == nil {
					b.HealthCheck = &HealthCheck{}
				}
				b.HealthCheck.Expect = joinText(w[2:])
			}
		case "timeout":
			if len(w) >= 3 && lineEnabled {
				if b.Timeouts == nil {
					b.Timeouts = map[string]string{}
				}
				b.Timeouts[w[1].Text] = w[2].Text
			}
		case "server":
			if sv, ok := parseServer(w); ok {
				sv.Line = i
				sv.Enabled = lineEnabled
				sv.Text = d.directiveText(i)
				b.Servers = append(b.Servers, sv)
			}
		}
	}
	return b
}

func joinText(w []Word) string {
	parts := make([]string, len(w))
	for i, x := range w {
		parts[i] = x.Text
	}
	return strings.Join(parts, " ")
}

// serverArgKeywords are server keywords this tool edits and that take one argument.
var serverArgKeywords = map[string]bool{"inter": true, "fall": true, "rise": true, "weight": true, "verify": true}

func parseServer(w []Word) (Server, bool) {
	if len(w) < 3 {
		return Server{}, false
	}
	s := Server{Name: w[1].Text}
	s.Address, s.Port = splitAddress(w[2].Text)
	rest := w[3:]
	for k := 0; k < len(rest); k++ {
		t := rest[k].Text
		switch t {
		case "check":
			s.Check = true
		case "disabled":
			s.Maintenance = true
		case "backup":
			s.Backup = true
		case "ssl":
			s.SSL = true
		default:
			if serverArgKeywords[t] && k+1 < len(rest) {
				v := rest[k+1].Text
				switch t {
				case "inter":
					s.Inter = v
				case "fall":
					s.Fall = v
				case "rise":
					s.Rise = v
				case "weight":
					s.Weight = v
				case "verify":
					s.Verify = v
				}
				k++
				continue
			}
			s.Extra = append(s.Extra, rest[k].Raw)
		}
	}
	return s, true
}

// splitAddress splits "host:port" into its parts. IPv6 addresses must use
// brackets or have a port after the last colon.
func splitAddress(a string) (string, string) {
	if strings.HasPrefix(a, "unix@") || strings.HasPrefix(a, "/") || strings.HasPrefix(a, "abns@") {
		return a, ""
	}
	if strings.HasPrefix(a, "[") {
		if k := strings.Index(a, "]"); k > 0 {
			host := a[1:k]
			if strings.HasPrefix(a[k+1:], ":") {
				return host, a[k+2:]
			}
			return host, ""
		}
	}
	k := strings.LastIndex(a, ":")
	if k < 0 {
		return a, ""
	}
	if strings.Count(a, ":") > 1 && !isPort(a[k+1:]) {
		return a, "" // bare IPv6 without port
	}
	return a[:k], a[k+1:]
}

func isPort(p string) bool {
	if p == "" {
		return false
	}
	for _, c := range p {
		if (c < '0' || c > '9') && c != '-' && c != '+' {
			return false
		}
	}
	return true
}

// joinAddress is the inverse of splitAddress.
func joinAddress(host, port string) string {
	if port == "" {
		return host
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}
