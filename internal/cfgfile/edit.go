package cfgfile

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Error is a user facing error with a stable code for translation.
type Error struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Params  map[string]string `json:"params,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func errf(code string, params map[string]string, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...), Params: params}
}

// Op is a single change. Only the fields relevant to the given Op are used.
type Op struct {
	Op           string        `json:"op"`
	Frontend     string        `json:"frontend,omitempty"`
	Backend      string        `json:"backend,omitempty"`
	Name         string        `json:"name,omitempty"`
	Kind         string        `json:"kind,omitempty"`
	Line         *int          `json:"line,omitempty"`
	Enabled      *bool         `json:"enabled,omitempty"`
	KeepBackend  bool          `json:"keepBackend,omitempty"`
	Server       *ServerSpec   `json:"server,omitempty"`
	BackendSpec  *BackendSpec  `json:"backendSpec,omitempty"`
	FrontendSpec *FrontendSpec `json:"frontendSpec,omitempty"`
	Route        *RouteSpec    `json:"route,omitempty"`
	Content      *string       `json:"content,omitempty"`
}

// ServerSpec describes a server line.
type ServerSpec struct {
	Name        string   `json:"name"`
	Address     string   `json:"address"`
	Port        string   `json:"port"`
	Check       bool     `json:"check"`
	Inter       string   `json:"inter,omitempty"`
	Fall        string   `json:"fall,omitempty"`
	Rise        string   `json:"rise,omitempty"`
	Weight      string   `json:"weight,omitempty"`
	Backup      bool     `json:"backup,omitempty"`
	SSL         bool     `json:"ssl,omitempty"`
	Verify      string   `json:"verify,omitempty"`
	Maintenance bool     `json:"maintenance,omitempty"`
	Extra       []string `json:"extra,omitempty"`
}

// BackendSpec describes a backend. On update, Servers is ignored.
type BackendSpec struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Mode        string            `json:"mode,omitempty"`
	Balance     string            `json:"balance,omitempty"`
	HealthCheck *HealthCheck      `json:"healthCheck,omitempty"`
	Timeouts    map[string]string `json:"timeouts,omitempty"`
	Servers     []ServerSpec      `json:"servers,omitempty"`
}

// FrontendSpec describes a new frontend.
type FrontendSpec struct {
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	Binds          []string `json:"binds"`
	DefaultBackend string   `json:"defaultBackend,omitempty"`
}

// RouteSpec describes a host/path based use_backend rule.
type RouteSpec struct {
	Frontend string   `json:"frontend"`
	Backend  string   `json:"backend"`
	Hosts    []string `json:"hosts,omitempty"`
	Paths    []string `json:"paths,omitempty"`
}

// Apply runs a list of operations on a copy of the document. The original is
// not modified.
func (d *Document) Apply(ops []Op) (*Document, error) {
	c := d.Clone()
	for k, op := range ops {
		if err := c.apply(op); err != nil {
			if e, ok := err.(*Error); ok {
				if e.Params == nil {
					e.Params = map[string]string{}
				}
				e.Params["opIndex"] = fmt.Sprint(k)
			}
			return nil, err
		}
	}
	return c, nil
}

func (d *Document) apply(op Op) error {
	switch op.Op {
	case "server.add":
		return d.serverAdd(op.Backend, op.Server)
	case "server.update":
		return d.serverUpdate(op.Backend, op.Name, op.Server)
	case "server.delete":
		return d.serverDelete(op.Backend, op.Name)
	case "server.setEnabled":
		return d.serverSetEnabled(op.Backend, op.Name, boolOr(op.Enabled, true))
	case "backend.create":
		return d.backendCreate(op.BackendSpec)
	case "backend.update":
		return d.backendUpdate(op.Name, op.BackendSpec)
	case "backend.delete":
		return d.backendDelete(op.Name)
	case "service.setEnabled":
		return d.serviceSetEnabled(op.Backend, boolOr(op.Enabled, true), op.KeepBackend)
	case "route.create":
		return d.routeCreate(op.Route)
	case "route.update":
		return d.routeUpdate(op.Frontend, intOr(op.Line), op.Route)
	case "route.delete":
		return d.routeDelete(op.Frontend, intOr(op.Line))
	case "route.setEnabled":
		return d.routeSetEnabled(op.Frontend, intOr(op.Line), boolOr(op.Enabled, true))
	case "frontend.create":
		return d.frontendCreate(op.FrontendSpec)
	case "frontend.setDefaultBackend":
		return d.frontendSetDefault(op.Frontend, op.Backend)
	case "section.replace":
		return d.sectionReplace(op.Kind, op.Name, op.Content)
	case "raw.replace":
		if op.Content == nil {
			return errf("missing_field", map[string]string{"field": "content"}, "content is required")
		}
		nd := Parse(*op.Content)
		*d = *nd
		return nil
	}
	return errf("unknown_op", map[string]string{"op": op.Op}, "unknown operation %q", op.Op)
}

func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

func intOr(i *int) int {
	if i == nil {
		return -1
	}
	return *i
}

// ---------------------------------------------------------------------------
// low level line editing

func (d *Document) replaceLine(i int, s string) {
	d.Lines[i] = s
	d.scan()
}

func (d *Document) insertLines(at int, lines ...string) {
	if at < 0 {
		at = 0
	}
	if at > len(d.Lines) {
		at = len(d.Lines)
	}
	nl := make([]string, 0, len(d.Lines)+len(lines))
	nl = append(nl, d.Lines[:at]...)
	nl = append(nl, lines...)
	nl = append(nl, d.Lines[at:]...)
	d.Lines = nl
	d.scan()
}

func (d *Document) deleteLines(from, to int) {
	if from < 0 || to > len(d.Lines) || from >= to {
		return
	}
	d.Lines = append(d.Lines[:from:from], d.Lines[to:]...)
	d.scan()
}

// deleteSet removes the given line indices.
func (d *Document) deleteSet(idx []int) {
	sort.Sort(sort.Reverse(sort.IntSlice(idx)))
	last := -1
	for _, i := range idx {
		if i == last {
			continue
		}
		last = i
		d.Lines = append(d.Lines[:i:i], d.Lines[i+1:]...)
	}
	d.scan()
}

// setWords rewrites line i with new words, keeping indentation, comment
// level and the trailing comment.
func (d *Document) setWords(i int, words []string) {
	orig := d.Lines[i]
	li := d.info[i]
	prefix := ""
	body := orig
	for k := 0; k < li.Depth; k++ {
		ind := indentOf(body)
		rest := body[len(ind):]
		rest = strings.TrimPrefix(rest, "#")
		sp := ""
		if strings.HasPrefix(rest, " ") {
			sp = " "
			rest = rest[1:]
		}
		prefix += ind + "#" + sp
		body = rest
	}
	ind := indentOf(body)
	_, cpos := splitLine(body)
	trailing := ""
	if cpos >= 0 {
		// keep the whitespace in front of the comment
		j := cpos
		for j > 0 && (body[j-1] == ' ' || body[j-1] == '\t') {
			j--
		}
		trailing = body[j:]
	}
	d.Lines[i] = prefix + ind + strings.Join(words, " ") + trailing
	d.scan()
}

// sectionByName returns an active section or commented block for a proxy.
func (d *Document) proxy(kind, name string) (*Section, error) {
	if s := d.FindSection(kind, name); s != nil {
		return s, nil
	}
	if s := d.FindBlock(kind, name); s != nil {
		return s, nil
	}
	return nil, errf("not_found", map[string]string{"kind": kind, "name": name}, "%s %q not found", kind, name)
}

// ---------------------------------------------------------------------------
// servers

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

func checkName(field, v string) error {
	if !namePattern.MatchString(v) {
		return errf("invalid_name", map[string]string{"field": field, "value": v}, "invalid %s %q: only letters, digits, '-', '_', '.' and ':' are allowed", field, v)
	}
	return nil
}

func (s *ServerSpec) words() ([]string, error) {
	if err := checkName("server name", s.Name); err != nil {
		return nil, err
	}
	addr := strings.TrimSpace(s.Address)
	if addr == "" || strings.ContainsAny(addr, " \t#") {
		return nil, errf("invalid_address", map[string]string{"value": addr}, "invalid server address %q", addr)
	}
	if s.Port != "" && !isPort(s.Port) {
		return nil, errf("invalid_port", map[string]string{"value": s.Port}, "invalid port %q", s.Port)
	}
	w := []string{"server", s.Name, joinAddress(addr, s.Port)}
	if s.Check {
		w = append(w, "check")
	}
	for _, kv := range [][2]string{{"inter", s.Inter}, {"fall", s.Fall}, {"rise", s.Rise}, {"weight", s.Weight}} {
		if v := strings.TrimSpace(kv[1]); v != "" {
			if strings.ContainsAny(v, " \t#") {
				return nil, errf("invalid_value", map[string]string{"field": kv[0], "value": v}, "invalid %s %q", kv[0], v)
			}
			w = append(w, kv[0], v)
		}
	}
	if s.Backup {
		w = append(w, "backup")
	}
	if s.SSL {
		w = append(w, "ssl")
		if s.Verify != "" {
			w = append(w, "verify", s.Verify)
		}
	}
	if s.Maintenance {
		w = append(w, "disabled")
	}
	for _, e := range s.Extra {
		if strings.TrimSpace(e) != "" {
			w = append(w, strings.TrimSpace(e))
		}
	}
	return w, nil
}

// findServer returns the line index of a server in a section.
func (d *Document) findServer(s *Section, name string) int {
	for _, i := range d.BodyLines(s) {
		li := d.info[i]
		if (li.Kind == KindDirective || li.Kind == KindDisabled) && li.Keyword() == "server" && len(li.Words) > 1 && li.Words[1].Text == name {
			return i
		}
	}
	return -1
}

func (d *Document) serverAdd(backend string, spec *ServerSpec) error {
	if spec == nil {
		return errf("missing_field", map[string]string{"field": "server"}, "server is required")
	}
	s, err := d.proxy("backend", backend)
	if err != nil {
		return err
	}
	if d.findServer(s, spec.Name) >= 0 {
		return errf("exists", map[string]string{"kind": "server", "name": spec.Name}, "server %q already exists in %q", spec.Name, backend)
	}
	w, err := spec.words()
	if err != nil {
		return err
	}
	line := d.Indent() + strings.Join(w, " ")
	// After the last server line, otherwise at the end of the section.
	at := -1
	for _, i := range d.BodyLines(s) {
		li := d.info[i]
		if (li.Kind == KindDirective || li.Kind == KindDisabled) && (li.Keyword() == "server" || li.Keyword() == "server-template") {
			at = i + 1
		}
	}
	if at < 0 {
		_, at = d.Extent(s)
	}
	if s.Disabled {
		line = commentOut(line)
	}
	d.insertLines(at, line)
	return nil
}

func (d *Document) serverUpdate(backend, name string, spec *ServerSpec) error {
	if spec == nil {
		return errf("missing_field", map[string]string{"field": "server"}, "server is required")
	}
	s, err := d.proxy("backend", backend)
	if err != nil {
		return err
	}
	i := d.findServer(s, name)
	if i < 0 {
		return errf("not_found", map[string]string{"kind": "server", "name": name}, "server %q not found in %q", name, backend)
	}
	if spec.Name != name && d.findServer(s, spec.Name) >= 0 {
		return errf("exists", map[string]string{"kind": "server", "name": spec.Name}, "server %q already exists in %q", spec.Name, backend)
	}
	w, err := spec.words()
	if err != nil {
		return err
	}
	d.setWords(i, w)
	return nil
}

func (d *Document) serverDelete(backend, name string) error {
	s, err := d.proxy("backend", backend)
	if err != nil {
		return err
	}
	i := d.findServer(s, name)
	if i < 0 {
		return errf("not_found", map[string]string{"kind": "server", "name": name}, "server %q not found in %q", name, backend)
	}
	d.deleteLines(i, i+1)
	return nil
}

// serverSetEnabled toggles the "disabled" keyword (maintenance mode that
// survives reloads). A commented-out server line is uncommented when enabled.
func (d *Document) serverSetEnabled(backend, name string, enabled bool) error {
	s, err := d.proxy("backend", backend)
	if err != nil {
		return err
	}
	i := d.findServer(s, name)
	if i < 0 {
		return errf("not_found", map[string]string{"kind": "server", "name": name}, "server %q not found in %q", name, backend)
	}
	li := d.info[i]
	blockDepth := 0
	if s.Disabled {
		blockDepth = d.info[s.Header].Depth
	}
	if enabled && li.Kind == KindDisabled && li.Depth > blockDepth {
		d.replaceLine(i, stripComment(d.Lines[i]))
		li = d.info[i]
	}
	var words []string
	has := false
	for _, w := range li.Words {
		if w.Text == "disabled" {
			has = true
			continue
		}
		words = append(words, w.Raw)
	}
	if !enabled {
		words = append(words, "disabled")
	}
	if has || !enabled {
		d.setWords(i, words)
	}
	return nil
}

// ---------------------------------------------------------------------------
// backends

func (b *BackendSpec) settingLines(indent string) ([]string, error) {
	var out []string
	if b.Mode != "" {
		if b.Mode != "http" && b.Mode != "tcp" {
			return nil, errf("invalid_value", map[string]string{"field": "mode", "value": b.Mode}, "invalid mode %q", b.Mode)
		}
		out = append(out, indent+"mode "+b.Mode)
	}
	if b.Balance != "" {
		out = append(out, indent+"balance "+b.Balance)
	}
	if hc := b.HealthCheck; hc != nil && (hc.Path != "" || hc.Method != "") {
		l := indent + "option httpchk"
		if hc.Method != "" {
			l += " " + hc.Method
		}
		if hc.Path != "" {
			l += " " + quoteWord(hc.Path)
		}
		out = append(out, l)
		if hc.Expect != "" {
			out = append(out, indent+"http-check expect "+hc.Expect)
		}
	}
	keys := make([]string, 0, len(b.Timeouts))
	for k := range b.Timeouts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := strings.TrimSpace(b.Timeouts[k]); v != "" {
			out = append(out, indent+"timeout "+k+" "+v)
		}
	}
	return out, nil
}

func (d *Document) backendCreate(spec *BackendSpec) error {
	if spec == nil {
		return errf("missing_field", map[string]string{"field": "backendSpec"}, "backendSpec is required")
	}
	if err := checkName("backend name", spec.Name); err != nil {
		return err
	}
	if d.FindSection("backend", spec.Name) != nil || d.FindBlock("backend", spec.Name) != nil {
		return errf("exists", map[string]string{"kind": "backend", "name": spec.Name}, "backend %q already exists", spec.Name)
	}
	ind := d.Indent()
	lines := []string{}
	for _, l := range descriptionLines(spec.Description) {
		lines = append(lines, l)
	}
	lines = append(lines, "backend "+spec.Name)
	set, err := spec.settingLines(ind)
	if err != nil {
		return err
	}
	lines = append(lines, set...)
	if len(spec.Servers) > 0 {
		if len(set) > 0 {
			lines = append(lines, "")
		}
		seen := map[string]bool{}
		for _, sv := range spec.Servers {
			if seen[sv.Name] {
				return errf("exists", map[string]string{"kind": "server", "name": sv.Name}, "duplicate server name %q", sv.Name)
			}
			seen[sv.Name] = true
			w, err := sv.words()
			if err != nil {
				return err
			}
			lines = append(lines, ind+strings.Join(w, " "))
		}
	}
	d.insertSection(lines, "backend")
	return nil
}

func descriptionLines(desc string) []string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return nil
	}
	var out []string
	for _, l := range strings.Split(desc, "\n") {
		l = strings.TrimRight(l, " \t\r")
		if l == "" {
			out = append(out, "#")
		} else {
			out = append(out, "# "+l)
		}
	}
	return out
}

// insertSection appends a new section after the last section of a similar
// kind (or at the end of the file), separated by a blank line.
func (d *Document) insertSection(lines []string, kind string) {
	at := len(d.Lines)
	last := -1
	all := append(append([]*Section{}, d.sections...), d.blocks...)
	for _, s := range all {
		if kindMatches(kind, s.Kind) || s.Kind == kind {
			_, e := d.Extent(s)
			if e > last {
				last = e
			}
		}
	}
	if last >= 0 {
		at = last
	}
	// Trim trailing blank lines at the insertion point when appending at EOF.
	block := []string{}
	if at > 0 && !isBlank(d.Lines[at-1]) {
		block = append(block, "")
	}
	block = append(block, lines...)
	if at < len(d.Lines) && !isBlank(d.Lines[at]) {
		block = append(block, "")
	}
	d.insertLines(at, block...)
}

// settingMatch returns a matcher for directives managed by backend.update.
func settingKey(w []Word) string {
	if len(w) == 0 {
		return ""
	}
	switch w[0].Text {
	case "mode", "balance":
		return w[0].Text
	case "option":
		if len(w) > 1 && w[1].Text == "httpchk" {
			return "option httpchk"
		}
	case "http-check":
		if len(w) > 1 && w[1].Text == "expect" {
			return "http-check expect"
		}
	case "timeout":
		if len(w) > 1 {
			return "timeout " + w[1].Text
		}
	}
	return ""
}

func (d *Document) backendUpdate(name string, spec *BackendSpec) error {
	if spec == nil {
		return errf("missing_field", map[string]string{"field": "backendSpec"}, "backendSpec is required")
	}
	s, err := d.proxy("backend", name)
	if err != nil {
		return err
	}
	want := map[string]string{}
	set, err := spec.settingLines("")
	if err != nil {
		return err
	}
	order := []string{}
	for _, l := range set {
		w, _ := splitLine(l)
		k := settingKey(w)
		want[k] = l
		order = append(order, k)
	}
	managed := []string{"mode", "balance", "option httpchk", "http-check expect"}
	for _, k := range []string{"tunnel", "server", "connect", "client", "queue", "check", "http-request", "http-keep-alive", "tarpit"} {
		managed = append(managed, "timeout "+k)
	}
	// Edit / delete existing lines.
	done := map[string]bool{}
	prefix := ""
	if s.Disabled {
		prefix = "# "
	}
	{
		var toDelete []int
		for _, i := range d.BodyLines(s) {
			li := d.info[i]
			if li.Kind != KindDirective && !(s.Disabled && li.Kind == KindDisabled && li.Depth == d.info[s.Header].Depth) {
				continue
			}
			k := settingKey(li.Words)
			if k == "" || !contains(managed, k) {
				continue
			}
			if l, ok := want[k]; ok && !done[k] {
				w, _ := splitLine(l)
				words := make([]string, len(w))
				for x := range w {
					words[x] = w[x].Raw
				}
				d.setWords(i, words)
				done[k] = true
			} else {
				toDelete = append(toDelete, i)
			}
		}
		if len(toDelete) > 0 {
			d.deleteSet(toDelete)
			s, _ = d.proxy("backend", name)
		}
	}
	// Insert missing settings after the last setting line before the first server.
	var missing []string
	for _, k := range order {
		if !done[k] {
			missing = append(missing, prefixLine(d.Indent(), prefix, want[k]))
		}
	}
	if len(missing) > 0 {
		at := s.Header + 1
		for _, i := range d.BodyLines(s) {
			li := d.info[i]
			if li.Kind == KindBlank || li.Kind == KindComment {
				continue
			}
			kw := li.Keyword()
			if kw == "server" || kw == "server-template" || kw == "default-server" {
				break
			}
			at = i + 1
		}
		d.insertLines(at, missing...)
	}
	return nil
}

func prefixLine(indent, commentPrefix, l string) string {
	if commentPrefix != "" {
		return indent + commentPrefix + l
	}
	return indent + l
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (d *Document) backendDelete(name string) error {
	s, err := d.proxy("backend", name)
	if err != nil {
		return err
	}
	if !s.Disabled {
		if refs := d.activeReferences(name); len(refs) > 0 {
			return errf("in_use", map[string]string{"name": name, "frontends": strings.Join(refs, ", ")}, "backend %q is still used by %s", name, strings.Join(refs, ", "))
		}
	}
	from, to := d.Extent(s)
	lf, lt := d.LeadingComment(s)
	if lt > lf {
		from = lf
	}
	// swallow one blank line in front to keep spacing tidy
	if from > 0 && isBlank(d.Lines[from-1]) && (to >= len(d.Lines) || isBlank(d.Lines[to])) {
		from--
	}
	d.deleteLines(from, to)
	return nil
}

// activeReferences lists frontends that actively route to a backend.
func (d *Document) activeReferences(backend string) []string {
	var out []string
	for _, f := range d.Model().Frontends {
		if !f.Enabled {
			continue
		}
		used := f.DefaultBackend == backend
		for _, r := range f.Rules {
			if r.Enabled && r.Backend == backend {
				used = true
			}
		}
		if used {
			out = append(out, f.Name)
		}
	}
	return out
}

// commentSection comments out an active section.
func (d *Document) commentSection(s *Section) {
	from, to := d.Extent(s)
	col0 := d.CommentColumnZero()
	for i := from; i < to; i++ {
		if !isBlank(d.Lines[i]) {
			d.Lines[i] = d.commentLine(d.Lines[i], col0)
		}
	}
	d.scan()
}

// uncommentBlock enables a commented-out section block. If the block sits in
// the middle of another section (active lines follow before the next header)
// it is moved to the end of the file so it does not capture those lines.
func (d *Document) uncommentBlock(b *Section) {
	safe := true
	for i := b.End; i < len(d.Lines); i++ {
		li := d.info[i]
		if li.Kind == KindDirective {
			safe = li.Header
			break
		}
	}
	var lines []string
	for _, m := range b.Members {
		if isBlank(d.Lines[m]) {
			lines = append(lines, d.Lines[m])
		} else {
			lines = append(lines, stripComment(d.Lines[m]))
		}
	}
	if safe {
		for k, m := range b.Members {
			d.Lines[m] = lines[k]
		}
		d.scan()
		return
	}
	d.deleteLines(b.Header, b.End)
	block := []string{}
	if len(d.Lines) > 0 && !isBlank(d.Lines[len(d.Lines)-1]) {
		block = append(block, "")
	}
	d.insertLines(len(d.Lines), append(block, lines...)...)
}

func (d *Document) serviceSetEnabled(backend string, enabled, keepBackend bool) error {
	if enabled {
		if b := d.FindBlock("backend", backend); b != nil && d.FindSection("backend", backend) == nil {
			d.uncommentBlock(b)
		}
		if d.FindSection("backend", backend) == nil {
			return errf("not_found", map[string]string{"kind": "backend", "name": backend}, "backend %q not found", backend)
		}
		// enable all rules that point to it
		for {
			changed := false
			for _, f := range d.Model().Frontends {
				if !f.Enabled {
					continue
				}
				for _, r := range f.Rules {
					if !r.Enabled && r.Backend == backend {
						if err := d.routeSetEnabled(f.Name, r.Line, true); err != nil {
							return err
						}
						changed = true
						break
					}
				}
				if changed {
					break
				}
			}
			if !changed {
				break
			}
		}
		return nil
	}
	s := d.FindSection("backend", backend)
	if s == nil {
		if d.FindBlock("backend", backend) != nil {
			return nil // already disabled
		}
		return errf("not_found", map[string]string{"kind": "backend", "name": backend}, "backend %q not found", backend)
	}
	for _, f := range d.Model().Frontends {
		if f.Enabled && f.DefaultBackend == backend && !keepBackend {
			return errf("is_default", map[string]string{"name": backend, "frontend": f.Name}, "backend %q is the default backend of frontend %q and cannot be disabled", backend, f.Name)
		}
	}
	var lines []int
	for _, f := range d.Model().Frontends {
		if !f.Enabled {
			continue
		}
		for _, r := range f.Rules {
			if r.Enabled && r.Backend == backend {
				lines = append(lines, r.Line)
			}
		}
	}
	for _, i := range lines {
		d.Lines[i] = commentOut(d.Lines[i])
	}
	d.scan()
	if !keepBackend {
		d.commentSection(d.FindSection("backend", backend))
	}
	return nil
}

// ---------------------------------------------------------------------------
// routes

var hostPattern = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]+)?$`)

func (r *RouteSpec) normalize() error {
	var hosts, paths []string
	for _, h := range r.Hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if !hostPattern.MatchString(h) {
			return errf("invalid_host", map[string]string{"value": h}, "invalid host name %q", h)
		}
		hosts = append(hosts, h)
	}
	for _, p := range r.Paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, " \t#") {
			return errf("invalid_path", map[string]string{"value": p}, "invalid path prefix %q (must start with /)", p)
		}
		paths = append(paths, p)
	}
	if len(hosts) == 0 && len(paths) == 0 {
		return errf("route_empty", nil, "a route needs at least one host or path")
	}
	r.Hosts, r.Paths = dedupe(hosts), dedupe(paths)
	return nil
}

func (r *RouteSpec) kind() string {
	switch {
	case len(r.Hosts) > 0 && len(r.Paths) > 0:
		return "host+path"
	case len(r.Hosts) > 0:
		return "host"
	}
	return "path"
}

var nonIdent = regexp.MustCompile(`[^a-z0-9]+`)

func aclBase(backend string) string {
	b := strings.ToLower(backend)
	for _, p := range []string{"backend-", "backend_", "be_", "be-", "bk_", "bk-"} {
		b = strings.TrimPrefix(b, p)
	}
	b = strings.Trim(nonIdent.ReplaceAllString(b, "_"), "_")
	if b == "" {
		b = "route"
	}
	return b
}

// uniqueACL returns an ACL name that is not used in the frontend.
func uniqueACL(f *Frontend, base string, reserved map[string]bool) string {
	used := map[string]bool{}
	for _, a := range f.ACLs {
		used[a.Name] = true
	}
	name := base
	for k := 2; used[name] && !reserved[name]; k++ {
		name = fmt.Sprintf("%s_%d", base, k)
	}
	return name
}

func (d *Document) frontendModel(name string) (*Frontend, *Section, error) {
	s := d.FindSection("frontend", name)
	if s == nil {
		if d.FindBlock("frontend", name) != nil {
			return nil, nil, errf("disabled", map[string]string{"kind": "frontend", "name": name}, "frontend %q is commented out", name)
		}
		return nil, nil, errf("not_found", map[string]string{"kind": "frontend", "name": name}, "frontend %q not found", name)
	}
	for _, f := range d.Model().Frontends {
		if f.Name == name && f.Enabled {
			return f, s, nil
		}
	}
	return nil, nil, errf("not_found", map[string]string{"kind": "frontend", "name": name}, "frontend %q not found", name)
}

// routeLines builds ACL lines and the condition for a route.
func routeLines(indent string, r *RouteSpec, hostACL, pathACL string) ([]string, string) {
	var acls []string
	var cond []string
	if len(r.Hosts) > 0 {
		for _, h := range r.Hosts {
			if strings.HasPrefix(h, "*.") {
				acls = append(acls, indent+"acl "+hostACL+" hdr_end(host) -i "+h[1:])
			} else {
				acls = append(acls, indent+"acl "+hostACL+" hdr(host) -i "+h)
			}
		}
		cond = append(cond, hostACL)
	}
	if len(r.Paths) > 0 {
		for _, p := range r.Paths {
			acls = append(acls, indent+"acl "+pathACL+" path_beg "+p)
		}
		cond = append(cond, pathACL)
	}
	return acls, strings.Join(cond, " ")
}

// aclInsertPoint returns where new ACL lines go in a frontend.
func (d *Document) aclInsertPoint(s *Section) int {
	at := -1
	first := -1
	for _, i := range d.BodyLines(s) {
		li := d.info[i]
		if li.Kind != KindDirective && li.Kind != KindDisabled {
			continue
		}
		switch li.Keyword() {
		case "acl":
			at = i + 1
		case "http-request", "use_backend", "default_backend", "tcp-request", "redirect":
			if first < 0 && li.Kind == KindDirective {
				first = i
			}
		}
	}
	if at >= 0 {
		return at
	}
	if first >= 0 {
		return first
	}
	_, e := d.Extent(s)
	return e
}

var kindRank = map[string]int{"host+path": 0, "host": 1, "path": 2, "other": 3, "always": 3}

// ruleInsertPoint finds the position for a new rule of the given kind so that
// more specific rules come first (host+path, host, path, others). A new path
// rule goes in front of the existing path rules: a new, longer prefix such as
// /api/v2/ must be checked before an existing /api/.
func (d *Document) ruleInsertPoint(f *Frontend, s *Section, kind string) int {
	rank := kindRank[kind]
	if kind == "path" {
		for _, r := range f.Rules {
			if r.Kind == "path" {
				return r.Line
			}
		}
	}
	after := -1
	before := -1
	for _, r := range f.Rules {
		rr, ok := kindRank[r.Kind]
		if !ok {
			rr = 3
		}
		if rr <= rank {
			after = r.Line
		} else if before < 0 {
			before = r.Line
		}
	}
	if after >= 0 && (before < 0 || after < before) {
		return after + 1
	}
	if before >= 0 {
		return before
	}
	if f.DefaultLine > 0 {
		return f.DefaultLine
	}
	_, e := d.Extent(s)
	return e
}

func (d *Document) routeCreate(r *RouteSpec) error {
	if r == nil {
		return errf("missing_field", map[string]string{"field": "route"}, "route is required")
	}
	if err := r.normalize(); err != nil {
		return err
	}
	if err := checkName("backend name", r.Backend); err != nil {
		return err
	}
	f, s, err := d.frontendModel(r.Frontend)
	if err != nil {
		return err
	}
	base := aclBase(r.Backend)
	hostACL := uniqueACL(f, "is_"+base+"_host", nil)
	pathACL := uniqueACL(f, "is_"+base+"_path", nil)
	acls, cond := routeLines(d.Indent(), r, hostACL, pathACL)
	ruleAt := d.ruleInsertPoint(f, s, r.kind())
	rule := d.Indent() + "use_backend " + r.Backend + " if " + cond
	aclAt := d.aclInsertPoint(s)
	// Insert the later position first so the earlier index stays valid.
	if ruleAt >= aclAt {
		d.insertLines(ruleAt, rule)
		d.insertLines(aclAt, acls...)
	} else {
		d.insertLines(aclAt, acls...)
		d.insertLines(ruleAt, rule)
	}
	return nil
}

func (d *Document) ruleAt(frontend string, line int) (*Frontend, *Rule, error) {
	f, _, err := d.frontendModel(frontend)
	if err != nil {
		return nil, nil, err
	}
	for k := range f.Rules {
		if f.Rules[k].Line == line {
			return f, &f.Rules[k], nil
		}
	}
	return nil, nil, errf("not_found", map[string]string{"kind": "route", "line": fmt.Sprint(line + 1)}, "no use_backend rule at line %d in frontend %q", line+1, frontend)
}

// exclusiveACLs returns ACL line indices referenced by rule r and by nothing
// else in the frontend.
func (d *Document) exclusiveACLs(f *Frontend, s *Section, r *Rule) ([]int, []string) {
	words, _ := splitLine(r.CondTest)
	names := map[string]bool{}
	for _, w := range words {
		names[strings.TrimPrefix(w.Text, "!")] = true
	}
	var lines []int
	var kept []string
	for name := range names {
		usedElsewhere := false
		for _, i := range d.BodyLines(s) {
			if i == r.Line {
				continue
			}
			li := d.info[i]
			if (li.Kind == KindDirective || li.Kind == KindDisabled) && li.Keyword() == "acl" && len(li.Words) > 1 && li.Words[1].Text == name {
				continue
			}
			for _, w := range li.Words {
				if strings.TrimPrefix(w.Text, "!") == name {
					usedElsewhere = true
				}
			}
		}
		if usedElsewhere {
			continue
		}
		found := false
		for _, a := range f.ACLs {
			if a.Name == name {
				lines = append(lines, a.Line)
				found = true
			}
		}
		if found {
			kept = append(kept, name)
		}
	}
	sort.Ints(lines)
	return lines, kept
}

func (d *Document) routeDelete(frontend string, line int) error {
	f, r, err := d.ruleAt(frontend, line)
	if err != nil {
		return err
	}
	s := d.FindSection("frontend", frontend)
	acl, _ := d.exclusiveACLs(f, s, r)
	d.deleteSet(append(acl, r.Line))
	return nil
}

func (d *Document) routeUpdate(frontend string, line int, spec *RouteSpec) error {
	if spec == nil {
		return errf("missing_field", map[string]string{"field": "route"}, "route is required")
	}
	if err := spec.normalize(); err != nil {
		return err
	}
	if err := checkName("backend name", spec.Backend); err != nil {
		return err
	}
	f, r, err := d.ruleAt(frontend, line)
	if err != nil {
		return err
	}
	if r.Kind == "other" || r.Kind == "always" {
		return errf("route_complex", map[string]string{"line": fmt.Sprint(line + 1)}, "the rule at line %d uses a complex condition; edit it in the config editor", line+1)
	}
	s := d.FindSection("frontend", frontend)
	oldACLs, names := d.exclusiveACLs(f, s, r)
	reserved := map[string]bool{}
	for _, n := range names {
		reserved[n] = true
	}
	// Reuse existing ACL names by their kind.
	hostACL, pathACL := "", ""
	for _, a := range f.ACLs {
		if !reserved[a.Name] {
			continue
		}
		if a.Kind == "host" && hostACL == "" {
			hostACL = a.Name
		} else if a.Kind == "path" && pathACL == "" && a.Name != hostACL {
			pathACL = a.Name
		}
	}
	base := aclBase(spec.Backend)
	if hostACL == "" {
		hostACL = uniqueACL(f, "is_"+base+"_host", nil)
	}
	if pathACL == "" {
		pathACL = uniqueACL(f, "is_"+base+"_path", nil)
		if pathACL == hostACL {
			pathACL = uniqueACL(f, "is_"+base+"_path_2", nil)
		}
	}
	wantACLs, cond := routeLines(d.Indent(), spec, hostACL, pathACL)
	// Rule line: only rewrite it when backend or condition change.
	if r.Backend != spec.Backend || strings.Join(strings.Fields(r.CondTest), " ") != cond {
		words := []string{"use_backend", spec.Backend, "if"}
		words = append(words, strings.Fields(cond)...)
		d.setWords(r.Line, words)
	}
	// ACL lines: keep the ones that are still wanted, delete the rest, add
	// the missing ones next to their siblings.
	key := func(l string) string {
		w, _ := splitLine(l)
		parts := make([]string, len(w))
		for k := range w {
			parts[k] = w[k].Text
		}
		return strings.Join(parts, " ")
	}
	want := map[string]string{}
	var order []string
	for _, l := range wantACLs {
		want[key(l)] = l
		order = append(order, key(l))
	}
	have := map[string]bool{}
	var del []int
	for _, i := range oldACLs {
		k := key(d.directiveText(i))
		if _, ok := want[k]; ok && !have[k] {
			have[k] = true
			continue
		}
		del = append(del, i)
	}
	firstOld := -1
	if len(oldACLs) > 0 {
		firstOld = oldACLs[0]
	}
	if len(del) > 0 {
		d.deleteSet(del)
		s = d.FindSection("frontend", frontend)
	}
	for _, k := range order {
		if have[k] {
			continue
		}
		l := want[k]
		if !r.Enabled {
			l = commentOut(l)
		}
		name := strings.Fields(k)[1]
		at := -1
		for _, i := range d.BodyLines(s) {
			li := d.info[i]
			if li.Keyword() == "acl" && len(li.Words) > 1 && li.Words[1].Text == name {
				at = i + 1
			}
		}
		if at < 0 {
			if firstOld >= 0 && firstOld <= len(d.Lines) {
				at = firstOld
			} else {
				at = d.aclInsertPoint(s)
			}
		}
		d.insertLines(at, l)
		s = d.FindSection("frontend", frontend)
	}
	return nil
}

func (d *Document) routeSetEnabled(frontend string, line int, enabled bool) error {
	f, r, err := d.ruleAt(frontend, line)
	if err != nil {
		return err
	}
	if r.Enabled == enabled {
		return nil
	}
	if !enabled {
		d.replaceLine(r.Line, commentOut(d.Lines[r.Line]))
		return nil
	}
	d.replaceLine(r.Line, stripComment(d.Lines[r.Line]))
	// Re-enable ACLs that are only available as commented-out lines.
	words, _ := splitLine(r.CondTest)
	var aclLines []int
	for _, w := range words {
		name := strings.TrimPrefix(w.Text, "!")
		active := false
		var disabled []int
		for _, a := range f.ACLs {
			if a.Name != name {
				continue
			}
			if a.Enabled {
				active = true
			} else {
				disabled = append(disabled, a.Line)
			}
		}
		if !active {
			aclLines = append(aclLines, disabled...)
		}
	}
	for _, i := range aclLines {
		d.Lines[i] = stripComment(d.Lines[i])
	}
	d.scan()
	// Enable the target backend if it is commented out.
	if d.FindSection("backend", r.Backend) == nil {
		if b := d.FindBlock("backend", r.Backend); b != nil {
			d.uncommentBlock(b)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// frontends and sections

func (d *Document) frontendCreate(spec *FrontendSpec) error {
	if spec == nil {
		return errf("missing_field", map[string]string{"field": "frontendSpec"}, "frontendSpec is required")
	}
	if err := checkName("frontend name", spec.Name); err != nil {
		return err
	}
	if d.FindSection("frontend", spec.Name) != nil || d.FindBlock("frontend", spec.Name) != nil {
		return errf("exists", map[string]string{"kind": "frontend", "name": spec.Name}, "frontend %q already exists", spec.Name)
	}
	if len(spec.Binds) == 0 {
		return errf("missing_field", map[string]string{"field": "binds"}, "a frontend needs at least one bind address")
	}
	ind := d.Indent()
	lines := descriptionLines(spec.Description)
	lines = append(lines, "frontend "+spec.Name)
	for _, b := range spec.Binds {
		b = strings.TrimSpace(b)
		if b == "" || strings.Contains(b, "#") {
			return errf("invalid_value", map[string]string{"field": "bind", "value": b}, "invalid bind %q", b)
		}
		lines = append(lines, ind+"bind "+b)
	}
	if spec.Mode != "" {
		lines = append(lines, ind+"mode "+spec.Mode)
	}
	if spec.DefaultBackend != "" {
		if err := checkName("backend name", spec.DefaultBackend); err != nil {
			return err
		}
		lines = append(lines, ind+"default_backend "+spec.DefaultBackend)
	}
	d.insertSection(lines, "frontend")
	return nil
}

func (d *Document) frontendSetDefault(frontend, backend string) error {
	f, s, err := d.frontendModel(frontend)
	if err != nil {
		return err
	}
	if backend == "" {
		if f.DefaultBackend != "" {
			d.deleteLines(f.DefaultLine, f.DefaultLine+1)
		}
		return nil
	}
	if err := checkName("backend name", backend); err != nil {
		return err
	}
	if f.DefaultBackend != "" {
		d.setWords(f.DefaultLine, []string{"default_backend", backend})
		return nil
	}
	_, e := d.Extent(s)
	d.insertLines(e, d.Indent()+"default_backend "+backend)
	return nil
}

// SectionText returns the text of a section (header to last own line).
func (d *Document) SectionText(kind, name string) (string, error) {
	s, err := d.proxy(kind, name)
	if err != nil {
		if kind == "global" || kind == "defaults" {
			for _, x := range d.sections {
				if x.Kind == kind && x.Name == name {
					s, err = x, nil
				}
			}
		}
		if err != nil {
			return "", err
		}
	}
	from, to := d.Extent(s)
	return strings.Join(d.Lines[from:to], "\n"), nil
}

func (d *Document) sectionReplace(kind, name string, content *string) error {
	if content == nil {
		return errf("missing_field", map[string]string{"field": "content"}, "content is required")
	}
	s, err := d.proxy(kind, name)
	if err != nil {
		for _, x := range d.sections {
			if x.Kind == kind && x.Name == name {
				s, err = x, nil
			}
		}
		if err != nil {
			return err
		}
	}
	from, to := d.Extent(s)
	text := strings.ReplaceAll(*content, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	nl := append([]string{}, d.Lines[:from]...)
	nl = append(nl, lines...)
	nl = append(nl, d.Lines[to:]...)
	d.Lines = nl
	d.scan()
	return nil
}
