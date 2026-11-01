// Package cfgfile implements a lossless, line-preserving reader and editor
// for HAProxy configuration files.
//
// Unlike a parse-and-regenerate approach, every edit only touches the lines it
// is about. Comments, blank lines, alignment and the order of sections stay
// exactly as the author wrote them. Commented-out directives and sections
// ("# use_backend …", "# backend …") are recognised and treated as disabled
// configuration, which mirrors how people manage HAProxy configs by hand.
package cfgfile

import "strings"

// Word is a single argument on a configuration line.
type Word struct {
	Text  string // value with quotes and escapes resolved
	Raw   string // the word exactly as written
	Start int    // byte offset of Raw in the line
	End   int    // byte offset after Raw
}

// splitLine tokenises a configuration line the way HAProxy does: words are
// separated by blanks, may be quoted with "…" or '…', backslash escapes the next
// character and an unquoted '#' starts a comment. It returns the words and the
// offset of the comment (or -1).
func splitLine(s string) ([]Word, int) {
	var words []Word
	i := 0
	n := len(s)
	for i < n {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '#' {
			return words, i
		}
		start := i
		var b strings.Builder
		for i < n {
			c = s[i]
			if c == ' ' || c == '\t' || c == '\r' {
				break
			}
			if c == '#' {
				break
			}
			switch c {
			case '\\':
				if i+1 < n {
					b.WriteByte(s[i+1])
					i += 2
				} else {
					i++
				}
			case '"':
				i++
				for i < n && s[i] != '"' {
					if s[i] == '\\' && i+1 < n {
						b.WriteByte(s[i+1])
						i += 2
						continue
					}
					b.WriteByte(s[i])
					i++
				}
				i++ // closing quote
			case '\'':
				i++
				for i < n && s[i] != '\'' {
					b.WriteByte(s[i])
					i++
				}
				i++
			default:
				b.WriteByte(c)
				i++
			}
		}
		if i > n {
			i = n
		}
		words = append(words, Word{Text: b.String(), Raw: s[start:i], Start: start, End: i})
	}
	return words, -1
}

// indentOf returns the leading whitespace of s.
func indentOf(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[:i]
		}
	}
	return s
}

// isBlank reports whether s only contains whitespace.
func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

// isCommentLine reports whether the first non-blank character of s is '#'.
func isCommentLine(s string) bool {
	t := strings.TrimLeft(s, " \t")
	return strings.HasPrefix(t, "#")
}

// stripComment removes one level of commenting from a comment line: the first
// '#' after the indentation and a single following space. The indentation in
// front of the '#' is kept, so both "    # balance x" and "#     balance x"
// become "    balance x".
func stripComment(s string) string {
	ind := indentOf(s)
	rest := s[len(ind):]
	if !strings.HasPrefix(rest, "#") {
		return s
	}
	rest = rest[1:]
	if strings.HasPrefix(rest, " ") {
		rest = rest[1:]
	}
	return ind + rest
}

// commentOut adds one level of commenting, keeping the indentation in front.
func commentOut(s string) string {
	if isBlank(s) {
		return s
	}
	ind := indentOf(s)
	return ind + "# " + s[len(ind):]
}

// quoteWord quotes a value for use in a config line if necessary.
func quoteWord(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \t#\"'\\") {
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
		return `"` + r.Replace(v) + `"`
	}
	return v
}
