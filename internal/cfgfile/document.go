package cfgfile

import (
	"strings"
)

// sectionKeywords are the keywords that open a new section.
var sectionKeywords = map[string]bool{
	"global": true, "defaults": true, "frontend": true, "backend": true,
	"listen": true, "userlist": true, "peers": true, "resolvers": true,
	"mailers": true, "program": true, "http-errors": true, "ring": true,
	"cache": true, "log-forward": true, "fcgi-app": true, "crt-store": true,
	"traces": true, "acme": true, "log-profile": true,
}

// directiveKeywords is a deliberately broad list of keywords that may appear
// inside sections. It is used to tell commented-out configuration apart from
// prose comments ("# Muss vor den Pfad-ACLs greifen" is prose,
// "# use_backend be if x" is configuration).
var directiveKeywords = map[string]bool{
	"acl": true, "balance": true, "bind": true, "mode": true, "option": true,
	"no": true, "http-check": true, "http-request": true, "http-response": true,
	"http-after-response": true, "http-error": true, "server": true,
	"server-template": true, "default-server": true, "timeout": true,
	"use_backend": true, "default_backend": true, "use-server": true,
	"cookie": true, "stick": true, "stick-table": true, "tcp-request": true,
	"tcp-response": true, "tcp-check": true, "redirect": true, "errorfile": true,
	"errorfiles": true, "log": true, "maxconn": true, "hash-type": true,
	"retries": true, "retry-on": true, "compression": true, "capture": true,
	"filter": true, "monitor-uri": true, "stats": true, "description": true,
	"disabled": true, "enabled": true, "fullconn": true, "id": true,
	"rate-limit": true, "unique-id-format": true, "unique-id-header": true,
	"email-alert": true, "external-check": true, "force-persist": true,
	"ignore-persist": true, "persist": true, "http-reuse": true, "source": true,
	"dispatch": true, "log-format": true, "log-tag": true, "user": true,
	"group": true, "peer": true, "nameserver": true, "mailer": true,
	"command": true, "daemon": true, "nbthread": true, "chroot": true,
	"pidfile": true, "ssl-default-bind-ciphers": true,
	"ssl-default-bind-options": true, "ssl-default-bind-ciphersuites": true,
	"ssl-default-server-options": true, "ssl-default-server-ciphers": true,
	"tune.ssl.default-dh-param": true, "ca-base": true, "crt-base": true,
	"http-send-name-header": true, "load-server-state-from-file": true,
	"declare": true, "max-keep-alive-queue": true, "use-fcgi-app": true,
}

// Kind of a line.
type LineKind int

const (
	KindBlank     LineKind = iota
	KindComment            // a comment that is not configuration (prose, separators)
	KindDirective          // an active configuration line
	KindDisabled           // a commented-out configuration line ("# server …")
)

// LineInfo is the classification of a single line.
type LineInfo struct {
	Kind LineKind
	// Words of the active directive, or of the commented-out directive for
	// KindDisabled lines.
	Words []Word
	// Depth of commenting for KindDisabled lines (1 = "# x", 2 = "# # x").
	Depth int
	// Header is set when the (possibly commented-out) line opens a section.
	Header bool
}

// Keyword returns the first word of the line or "".
func (l LineInfo) Keyword() string {
	if len(l.Words) == 0 {
		return ""
	}
	return l.Words[0].Text
}

// Document is a configuration file kept as its exact lines.
type Document struct {
	Lines         []string
	CRLF          bool // the file used \r\n line endings
	FinalNewline  bool // the file ended with a newline
	info          []LineInfo
	sections      []*Section
	blocks        []*Section
	lineToSection []int // index into sections, -1 for preamble
	inBlock       []int // index into blocks, -1 if not part of a commented block
}

// Section is either an active section (header and body lines up to the next
// header) or a commented-out section block.
type Section struct {
	Kind     string // frontend, backend, listen, global, defaults, …
	Name     string
	Header   int  // line index of the header
	End      int  // exclusive end of the range (active: next header/EOF; block: last member+1)
	Disabled bool // commented-out block
	// Members of a commented-out block in order (blank lines included).
	Members []int
}

// Parse reads a configuration file.
func Parse(content string) *Document {
	d := &Document{}
	if strings.Contains(content, "\r\n") {
		d.CRLF = true
		content = strings.ReplaceAll(content, "\r\n", "\n")
	}
	d.FinalNewline = strings.HasSuffix(content, "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" && !d.FinalNewline {
		d.Lines = nil
	} else {
		d.Lines = strings.Split(content, "\n")
	}
	d.scan()
	return d
}

// String renders the document back to text.
func (d *Document) String() string {
	nl := "\n"
	if d.CRLF {
		nl = "\r\n"
	}
	s := strings.Join(d.Lines, nl)
	if d.FinalNewline || len(d.Lines) > 0 {
		s += nl
	}
	return s
}

// Clone returns an independent copy.
func (d *Document) Clone() *Document {
	c := &Document{CRLF: d.CRLF, FinalNewline: d.FinalNewline}
	c.Lines = append([]string(nil), d.Lines...)
	c.scan()
	return c
}

// Info returns the classification of line i.
func (d *Document) Info(i int) LineInfo { return d.info[i] }

// classify analyses one line.
func classify(s string) LineInfo {
	if isBlank(s) {
		return LineInfo{Kind: KindBlank}
	}
	if !isCommentLine(s) {
		w, _ := splitLine(s)
		li := LineInfo{Kind: KindDirective, Words: w}
		li.Header = len(w) > 0 && sectionKeywords[w[0].Text]
		return li
	}
	// Peel comment levels until we hit configuration or prose.
	cur := s
	for depth := 1; depth <= 4; depth++ {
		cur = stripComment(cur)
		if isBlank(cur) {
			return LineInfo{Kind: KindComment}
		}
		if isCommentLine(cur) {
			continue
		}
		w, _ := splitLine(cur)
		if len(w) == 0 {
			return LineInfo{Kind: KindComment}
		}
		kw := w[0].Text
		if sectionKeywords[kw] && looksLikeHeader(w) {
			return LineInfo{Kind: KindDisabled, Words: w, Depth: depth, Header: true}
		}
		if directiveKeywords[kw] && looksLikeDirective(w) {
			return LineInfo{Kind: KindDisabled, Words: w, Depth: depth}
		}
		return LineInfo{Kind: KindComment}
	}
	return LineInfo{Kind: KindComment}
}

// validName reports whether s is a valid HAProxy proxy/server/ACL name.
func validName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':') {
			return false
		}
	}
	return true
}

// looksLikeHeader filters out prose such as "# backend pools are …".
func looksLikeHeader(w []Word) bool {
	switch w[0].Text {
	case "global":
		return len(w) == 1
	case "defaults":
		return len(w) <= 4
	}
	if len(w) < 2 || !validName(w[1].Text) || strings.HasSuffix(w[1].Text, ":") {
		return false
	}
	// "backend name" or "backend name from defaults"
	return len(w) == 2 || (len(w) == 4 && w[2].Text == "from")
}

// looksLikeDirective applies a few plausibility checks for commented lines.
func looksLikeDirective(w []Word) bool {
	kw := w[0].Text
	if strings.HasSuffix(kw, ":") {
		return false
	}
	switch kw {
	case "server", "acl":
		return len(w) >= 3 && validName(w[1].Text)
	case "use_backend":
		return len(w) >= 2 && validName(w[1].Text) && (len(w) == 2 || w[2].Text == "if" || w[2].Text == "unless")
	case "default_backend":
		return len(w) == 2 && validName(w[1].Text)
	case "balance", "mode", "bind", "option", "http-check", "timeout", "http-request", "http-response", "cookie", "stats":
		return len(w) >= 2
	}
	return true
}

// scan (re)computes line classifications, sections and commented blocks.
func (d *Document) scan() {
	n := len(d.Lines)
	d.info = make([]LineInfo, n)
	for i, l := range d.Lines {
		d.info[i] = classify(l)
	}
	d.sections = nil
	d.blocks = nil
	d.lineToSection = make([]int, n)
	d.inBlock = make([]int, n)
	cur := -1
	for i := 0; i < n; i++ {
		d.inBlock[i] = -1
		li := d.info[i]
		if li.Kind == KindDirective && li.Header {
			s := &Section{Kind: li.Words[0].Text, Header: i, End: n}
			if len(li.Words) > 1 {
				s.Name = li.Words[1].Text
			}
			if cur >= 0 {
				d.sections[cur].End = i
			}
			d.sections = append(d.sections, s)
			cur = len(d.sections) - 1
		}
		d.lineToSection[i] = cur
	}
	// Commented-out section blocks.
	for i := 0; i < n; i++ {
		li := d.info[i]
		if li.Kind != KindDisabled || !li.Header || d.inBlock[i] >= 0 {
			continue
		}
		b := &Section{Kind: li.Words[0].Text, Header: i, Disabled: true}
		if len(li.Words) > 1 {
			b.Name = li.Words[1].Text
		}
		b.Members = []int{i}
		last := i
		for j := i + 1; j < n; j++ {
			lj := d.info[j]
			if lj.Kind == KindBlank {
				continue
			}
			if lj.Kind == KindDisabled && !lj.Header && lj.Depth >= li.Depth {
				for k := last + 1; k <= j; k++ {
					b.Members = append(b.Members, k)
				}
				last = j
				continue
			}
			// A nested prose comment ("# # explanation") belongs to a
			// section we commented out ourselves.
			if lj.Kind == KindComment && commentDepth(d.Lines[j]) > li.Depth {
				for k := last + 1; k <= j; k++ {
					b.Members = append(b.Members, k)
				}
				last = j
				continue
			}
			break
		}
		b.End = last + 1
		idx := len(d.blocks)
		d.blocks = append(d.blocks, b)
		for _, m := range b.Members {
			d.inBlock[m] = idx
		}
	}
}

// commentDepth counts how many comment levels a line has.
func commentDepth(s string) int {
	depth := 0
	for isCommentLine(s) && depth < 8 {
		s = stripComment(s)
		depth++
	}
	return depth
}

// Sections returns the active sections in file order.
func (d *Document) Sections() []*Section { return d.sections }

// Blocks returns the commented-out section blocks in file order.
func (d *Document) Blocks() []*Section { return d.blocks }

// FindSection returns the active section with the given kind and name.
// kind may be "proxy" to match frontend, backend and listen with a name.
func (d *Document) FindSection(kind, name string) *Section {
	for _, s := range d.sections {
		if s.Name == name && kindMatches(kind, s.Kind) {
			return s
		}
	}
	return nil
}

// FindBlock returns a commented-out section block.
func (d *Document) FindBlock(kind, name string) *Section {
	for _, s := range d.blocks {
		if s.Name == name && kindMatches(kind, s.Kind) {
			return s
		}
	}
	return nil
}

func kindMatches(want, have string) bool {
	if want == have {
		return true
	}
	switch want {
	case "backend":
		return have == "listen"
	case "frontend":
		return have == "listen"
	case "proxy":
		return have == "frontend" || have == "backend" || have == "listen"
	}
	return false
}

// BodyLines returns the line indices that belong to an active section's own
// body: everything after the header up to the last own line, excluding lines
// of commented-out section blocks and the leading comment block of the next
// section.
func (d *Document) BodyLines(s *Section) []int {
	if s.Disabled {
		return s.Members[1:]
	}
	end := d.ownEnd(s)
	var out []int
	for i := s.Header + 1; i < end; i++ {
		if d.inBlock[i] >= 0 {
			continue
		}
		out = append(out, i)
	}
	return out
}

// ownEnd returns the exclusive end of the lines that belong to section s:
// after the last active directive, extended by directly following
// commented-out directives (e.g. a disabled server at the very end).
func (d *Document) ownEnd(s *Section) int {
	if s.Disabled {
		return s.End
	}
	last := s.Header
	for i := s.Header + 1; i < s.End; i++ {
		if d.info[i].Kind == KindDirective && d.inBlock[i] < 0 {
			last = i
		}
	}
	for i := last + 1; i < s.End; i++ {
		if d.info[i].Kind == KindDisabled && !d.info[i].Header && d.inBlock[i] < 0 {
			last = i
			continue
		}
		break
	}
	return last + 1
}

// Extent returns the [from, to) range of lines that belong to the section
// (header through its last own line).
func (d *Document) Extent(s *Section) (int, int) {
	if s.Disabled {
		return s.Header, s.End
	}
	return s.Header, d.ownEnd(s)
}

// LeadingComment returns the range of the comment block directly describing a
// section (the "#----\n# Backend: …\n#----" banner above it), possibly
// separated from the header by blank lines.
func (d *Document) LeadingComment(s *Section) (int, int) {
	i := s.Header - 1
	for i >= 0 && d.info[i].Kind == KindBlank {
		i--
	}
	end := i + 1
	for i >= 0 && (d.info[i].Kind == KindComment || d.info[i].Kind == KindDisabled) && !d.inProxyBlock(i) {
		i--
	}
	from := i + 1
	if from >= end {
		return s.Header, s.Header
	}
	// Do not steal comments that are part of the previous section's own lines.
	if prev := d.lineToSection[from]; prev >= 0 && !s.Disabled {
		ps := d.sections[prev]
		if ps != s && d.ownEnd(ps) > from {
			return s.Header, s.Header
		}
	}
	return from, end
}

// Description extracts a human readable description from a section's leading
// comment, dropping separator lines.
func (d *Document) Description(s *Section) string {
	from, to := d.LeadingComment(s)
	var out []string
	for i := from; i < to; i++ {
		t := strings.TrimSpace(d.Lines[i])
		t = strings.TrimLeft(t, "#")
		t = strings.TrimSpace(t)
		if isSeparator(t) {
			continue
		}
		out = append(out, t)
	}
	// Trim empty lines at both ends.
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// inProxyBlock reports whether line i belongs to a commented-out frontend,
// backend or listen section.
func (d *Document) inProxyBlock(i int) bool {
	b := d.inBlock[i]
	if b < 0 {
		return false
	}
	switch d.blocks[b].Kind {
	case "frontend", "backend", "listen":
		return true
	}
	return false
}

func isSeparator(t string) bool {
	if len(t) < 3 {
		return false
	}
	for _, c := range t {
		if c != '-' && c != '=' && c != '#' && c != '*' && c != '~' && c != '_' {
			return false
		}
	}
	return true
}

// Indent returns the indentation used by the document for section bodies.
func (d *Document) Indent() string {
	counts := map[string]int{}
	for i, li := range d.info {
		if li.Kind == KindDirective && !li.Header {
			ind := indentOf(d.Lines[i])
			if ind != "" {
				counts[ind]++
			}
		}
	}
	best, bestN := "    ", 0
	for k, v := range counts {
		if v > bestN || (v == bestN && k < best) {
			best, bestN = k, v
		}
	}
	return best
}

// CommentColumnZero reports whether the document mostly comments out
// indented configuration with the '#' in column 0 ("#     server …") rather
// than after the indentation ("    # server …").
func (d *Document) CommentColumnZero() bool {
	col0, inner := 0, 0
	for i, li := range d.info {
		if li.Kind != KindDisabled || li.Header {
			continue
		}
		l := d.Lines[i]
		if strings.HasPrefix(l, "#") {
			if len(stripComment(l)) > 0 && (stripComment(l)[0] == ' ' || stripComment(l)[0] == '\t') {
				col0++
			}
		} else {
			inner++
		}
	}
	return col0 > inner
}

// commentLine comments out line i in the document's preferred style.
func (d *Document) commentLine(s string, col0 bool) string {
	if col0 && !isBlank(s) && indentOf(s) != "" {
		return "# " + s
	}
	return commentOut(s)
}

// directiveText returns the text of a line without commenting, for display.
func (d *Document) directiveText(i int) string {
	s := d.Lines[i]
	for k := 0; k < d.info[i].Depth; k++ {
		s = stripComment(s)
	}
	return strings.TrimSpace(s)
}
