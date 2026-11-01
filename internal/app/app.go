// Package app ties the configuration editor, validation, history and the
// HAProxy runtime together. Every change follows the same path:
//
//	read file → apply operations → diff → haproxy -c → write → snapshot → reload/runtime
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/toolvibes/sluiceops/internal/cfgfile"
	"github.com/toolvibes/sluiceops/internal/diff"
	"github.com/toolvibes/sluiceops/internal/haproxy"
	"github.com/toolvibes/sluiceops/internal/history"
)

// App is the central service.
type App struct {
	ConfigPath string
	ReadOnly   bool
	Validator  *haproxy.Validator
	Runtime    *haproxy.Runtime
	History    *history.Store
	mu         sync.Mutex
}

// ErrConflict is returned when the file changed since the client loaded it.
var ErrConflict = errors.New("the configuration was changed in the meantime; reload and try again")

// ErrReadOnly is returned for writes in read-only mode.
var ErrReadOnly = errors.New("SluiceOps runs in read-only mode")

// Snapshot is the current state of the configuration file.
type Snapshot struct {
	Version string            `json:"version"`
	Path    string            `json:"path"`
	Content string            `json:"-"`
	Doc     *cfgfile.Document `json:"-"`
	ModTime time.Time         `json:"modTime"`
}

// Load reads the configuration file. If it was changed outside SluiceOps since
// the last stored version, the new content is recorded in the history.
func (a *App) Load() (*Snapshot, error) {
	b, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		return nil, err
	}
	st, _ := os.Stat(a.ConfigPath)
	content := string(b)
	s := &Snapshot{Version: history.Hash(content), Path: a.ConfigPath, Content: content, Doc: cfgfile.Parse(content)}
	if st != nil {
		s.ModTime = st.ModTime()
	}
	if a.History != nil {
		a.recordExternal(content)
	}
	return s, nil
}

func (a *App) recordExternal(content string) {
	latest, err := a.History.Latest()
	if err != nil {
		return
	}
	if latest == nil {
		_, _ = a.History.Save(content, history.Entry{Kind: "initial", Message: "initial version"})
		return
	}
	if latest.Hash == history.Hash(content) {
		return
	}
	_, prev, _ := a.History.Get(latest.ID)
	d := diff.Compute(prev, content, 0)
	_, _ = a.History.Save(content, history.Entry{Kind: "external", Message: "changed outside SluiceOps", Added: d.Added, Removed: d.Removed})
}

// ChangeRequest is a list of operations based on a known version.
type ChangeRequest struct {
	Version string       `json:"version"`
	Ops     []cfgfile.Op `json:"ops"`
	Message string       `json:"message,omitempty"`
	// Reload controls reloading after a successful write: "auto" (default)
	// reloads only when needed, "never" only writes the file.
	Reload string `json:"reload,omitempty"`
}

// Preview is the outcome of a dry run.
type Preview struct {
	Version        string             `json:"version"`
	NewVersion     string             `json:"newVersion"`
	Diff           diff.Result        `json:"diff"`
	Validation     haproxy.Validation `json:"validation"`
	ReloadRequired bool               `json:"reloadRequired"`
	RuntimeActions []RuntimeAction    `json:"runtimeActions"`
	Changed        bool               `json:"changed"`
	content        string
}

// RuntimeAction is a live change that replaces a reload.
type RuntimeAction struct {
	Backend string `json:"backend"`
	Server  string `json:"server"`
	State   string `json:"state"`
	Error   string `json:"error,omitempty"`
}

// Result of an applied change.
type Result struct {
	Preview
	Entry        *history.Entry `json:"entry,omitempty"`
	Reloaded     bool           `json:"reloaded"`
	ReloadOutput string         `json:"reloadOutput,omitempty"`
	ReloadError  string         `json:"reloadError,omitempty"`
}

// plan computes the new content and what is needed to activate it.
func (a *App) plan(ctx context.Context, snap *Snapshot, req ChangeRequest) (*Preview, error) {
	if req.Version != "" && req.Version != snap.Version {
		return nil, ErrConflict
	}
	nd, err := snap.Doc.Apply(req.Ops)
	if err != nil {
		return nil, err
	}
	content := nd.String()
	p := &Preview{Version: snap.Version, NewVersion: history.Hash(content), content: content, RuntimeActions: []RuntimeAction{}}
	p.Changed = content != snap.Content
	p.Diff = diff.Compute(snap.Content, content, 3)
	if p.Changed {
		p.Validation = a.Validator.Check(ctx, content)
	} else {
		p.Validation = haproxy.Validation{Valid: true, Messages: []haproxy.Message{}, Output: "no changes"}
	}
	p.ReloadRequired, p.RuntimeActions = a.activation(snap.Doc, req.Ops)
	if !p.Changed {
		p.ReloadRequired = false
	}
	return p, nil
}

// activation decides whether the change can be activated through the runtime
// API (server maintenance toggles) or needs a reload.
func (a *App) activation(before *cfgfile.Document, ops []cfgfile.Op) (bool, []RuntimeAction) {
	actions := []RuntimeAction{}
	if a.Runtime == nil || !a.Runtime.Available() {
		return true, actions
	}
	m := before.Model()
	for _, op := range ops {
		if op.Op != "server.setEnabled" {
			return true, nil
		}
		live := false
		for _, b := range m.Backends {
			if b.Name != op.Backend || !b.Enabled {
				continue
			}
			for _, s := range b.Servers {
				if s.Name == op.Name && s.Enabled {
					live = true
				}
			}
		}
		if !live {
			return true, nil // the server does not exist in the running process
		}
		state := "ready"
		if op.Enabled != nil && !*op.Enabled {
			state = "maint"
		}
		actions = append(actions, RuntimeAction{Backend: op.Backend, Server: op.Name, State: state})
	}
	return false, actions
}

// Preview runs a change without writing anything.
func (a *App) Preview(ctx context.Context, req ChangeRequest) (*Preview, error) {
	snap, err := a.Load()
	if err != nil {
		return nil, err
	}
	return a.plan(ctx, snap, req)
}

// Apply validates and writes a change, records it and activates it.
func (a *App) Apply(ctx context.Context, user string, req ChangeRequest) (*Result, error) {
	if a.ReadOnly {
		return nil, ErrReadOnly
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	snap, err := a.Load()
	if err != nil {
		return nil, err
	}
	p, err := a.plan(ctx, snap, req)
	if err != nil {
		return nil, err
	}
	res := &Result{Preview: *p}
	if !p.Changed {
		return res, nil
	}
	if !p.Validation.Valid {
		return res, &ValidationError{Validation: p.Validation}
	}
	if err := writeInPlace(a.ConfigPath, p.content); err != nil {
		return nil, fmt.Errorf("cannot write %s: %w", a.ConfigPath, err)
	}
	kind := "change"
	for _, op := range req.Ops {
		if op.Op == "raw.replace" {
			kind = "raw"
		}
	}
	if strings.HasPrefix(req.Message, "rollback") {
		kind = "rollback"
	}
	entry := history.Entry{User: user, Message: req.Message, Kind: kind, Ops: summarize(req.Ops), Added: p.Diff.Added, Removed: p.Diff.Removed}
	// Activate.
	if req.Reload != "never" {
		if p.ReloadRequired {
			if a.Runtime != nil && a.Runtime.CanReload() {
				out, err := a.Runtime.Reload(ctx)
				res.ReloadOutput = out
				if err != nil {
					res.ReloadError = err.Error()
				} else {
					res.Reloaded = true
				}
			} else {
				res.ReloadError = "no reload method configured – reload HAProxy manually"
			}
		} else {
			for k, act := range res.RuntimeActions {
				if err := a.Runtime.SetServerState(act.Backend, act.Server, act.State); err != nil {
					res.RuntimeActions[k].Error = err.Error()
				}
			}
		}
	}
	entry.Reloaded = res.Reloaded
	if a.History != nil {
		e, err := a.History.Save(p.content, entry)
		if err == nil {
			res.Entry = &e
		}
	}
	return res, nil
}

// ValidationError carries the failed validation.
type ValidationError struct{ Validation haproxy.Validation }

func (e *ValidationError) Error() string { return "haproxy rejected the configuration" }

func summarize(ops []cfgfile.Op) []string {
	var out []string
	for _, op := range ops {
		s := op.Op
		switch {
		case op.Backend != "" && op.Name != "":
			s += " " + op.Backend + "/" + op.Name
		case op.Backend != "":
			s += " " + op.Backend
		case op.Name != "":
			s += " " + op.Name
		case op.BackendSpec != nil:
			s += " " + op.BackendSpec.Name
		case op.Route != nil:
			s += " " + op.Route.Frontend + "→" + op.Route.Backend
		case op.FrontendSpec != nil:
			s += " " + op.FrontendSpec.Name
		case op.Frontend != "":
			s += " " + op.Frontend
		}
		if op.Enabled != nil {
			if *op.Enabled {
				s += " on"
			} else {
				s += " off"
			}
		}
		out = append(out, s)
	}
	return out
}

// writeInPlace overwrites the file without replacing its inode. Atomic
// rename would break single-file bind mounts (Docker), which are common for
// haproxy.cfg. The previous content is in the history in case of a crash.
func writeInPlace(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Rollback restores a stored version.
func (a *App) Rollback(ctx context.Context, user string, id int64, version string) (*Result, error) {
	_, content, err := a.History.Get(id)
	if err != nil {
		return nil, err
	}
	return a.Apply(ctx, user, ChangeRequest{
		Version: version,
		Ops:     []cfgfile.Op{{Op: "raw.replace", Content: &content}},
		Message: fmt.Sprintf("rollback to version %d", id),
	})
}

// Status combines runtime information with the configuration model.
type Status struct {
	Available bool           `json:"available"`
	Error     string         `json:"error,omitempty"`
	Info      *haproxy.Info  `json:"info,omitempty"`
	Stats     []haproxy.Stat `json:"stats"`
}

// Status queries the runtime API.
func (a *App) Status() Status {
	st := Status{Stats: []haproxy.Stat{}}
	if a.Runtime == nil || !a.Runtime.Available() {
		st.Error = "no runtime socket configured"
		return st
	}
	info, err := a.Runtime.ShowInfo()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Available = true
	st.Info = info
	stats, err := a.Runtime.ShowStat()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Stats = stats
	return st
}

// SetServerState changes a server state at runtime only (drain etc.).
func (a *App) SetServerState(backend, server, state string) error {
	if a.ReadOnly {
		return ErrReadOnly
	}
	if a.Runtime == nil || !a.Runtime.Available() {
		return haproxy.ErrNoRuntime
	}
	return a.Runtime.SetServerState(backend, server, state)
}
