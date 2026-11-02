// Package haproxy talks to the HAProxy binary (validation) and to a running
// HAProxy instance (runtime API, reload).
package haproxy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Message is a single warning or alert from "haproxy -c".
type Message struct {
	Level   string `json:"level"` // alert, warning, notice
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// Validation is the result of checking a configuration.
type Validation struct {
	Valid     bool      `json:"valid"`
	Skipped   bool      `json:"skipped,omitempty"`
	Output    string    `json:"output"`
	Messages  []Message `json:"messages"`
	Duration  string    `json:"duration"`
	Validator string    `json:"validator,omitempty"`
}

// Validator runs "haproxy -c" on candidate configurations.
type Validator struct {
	Binary     string // path or name of the haproxy binary; empty disables validation
	ConfigPath string // the real config path; used as working dir and in messages
	Timeout    time.Duration
}

var lineRef = regexp.MustCompile(`\[[^\]]*?:(\d+)\]`)

// Check validates content as if it were the config file.
func (v *Validator) Check(ctx context.Context, content string) Validation {
	start := time.Now()
	if v.Binary == "" {
		return Validation{Valid: true, Skipped: true, Messages: []Message{}, Output: "validation disabled"}
	}
	dir := filepath.Dir(v.ConfigPath)
	tmp, err := os.CreateTemp(dir, ".sluvio-check-*.cfg")
	if err != nil {
		// Fall back to the system temp dir if the config dir is read-only.
		tmp, err = os.CreateTemp("", "sluvio-check-*.cfg")
		if err != nil {
			return Validation{Valid: false, Output: err.Error(), Messages: []Message{{Level: "alert", Message: err.Error()}}}
		}
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return Validation{Valid: false, Output: err.Error(), Messages: []Message{{Level: "alert", Message: err.Error()}}}
	}
	tmp.Close()
	to := v.Timeout
	if to == 0 {
		to = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	cmd := exec.CommandContext(ctx, v.Binary, "-c", "-f", tmp.Name())
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	text := strings.ReplaceAll(out.String(), tmp.Name(), v.ConfigPath)
	text = strings.ReplaceAll(text, filepath.Base(tmp.Name()), filepath.Base(v.ConfigPath))
	res := Validation{
		Valid:     runErr == nil,
		Output:    strings.TrimSpace(text),
		Messages:  parseMessages(text),
		Duration:  time.Since(start).Round(time.Millisecond).String(),
		Validator: v.Binary,
	}
	if runErr != nil && len(res.Messages) == 0 {
		res.Messages = []Message{{Level: "alert", Message: strings.TrimSpace(runErr.Error() + " " + text)}}
	}
	return res
}

func parseMessages(text string) []Message {
	msgs := []Message{}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		level := ""
		switch {
		case strings.Contains(l, "[ALERT]"):
			level = "alert"
		case strings.Contains(l, "[WARNING]"):
			level = "warning"
		case strings.Contains(l, "[NOTICE]"):
			level = "notice"
		default:
			continue
		}
		m := Message{Level: level, Message: l}
		if k := strings.Index(l, ": "); k >= 0 {
			// strip "[ALERT]    (1234) : " prefix
			if j := strings.Index(l, ") : "); j >= 0 {
				m.Message = strings.TrimSpace(l[j+4:])
			}
		}
		if sm := lineRef.FindStringSubmatch(l); sm != nil {
			m.Line, _ = strconv.Atoi(sm[1])
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// Version returns the version string of the binary.
func (v *Validator) Version(ctx context.Context) (string, error) {
	if v.Binary == "" {
		return "", fmt.Errorf("no haproxy binary configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, v.Binary, "-v").CombinedOutput()
	if err != nil {
		return "", err
	}
	first := strings.SplitN(string(out), "\n", 2)[0]
	return strings.TrimSpace(first), nil
}
