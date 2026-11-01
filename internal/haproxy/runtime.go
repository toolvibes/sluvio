package haproxy

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Runtime talks to the HAProxy runtime API. It can use the master socket of a
// master-worker HAProxy (recommended: "haproxy -W -S /run/haproxy/master.sock")
// which also allows reloads, or a plain "stats socket … level admin".
type Runtime struct {
	MasterSocket string // unix path or host:port
	StatsSocket  string // unix path or host:port
	ReloadCmd    string // optional shell command used for reloads
	Timeout      time.Duration
}

// ErrNoRuntime is returned when no socket is configured.
var ErrNoRuntime = errors.New("no runtime socket configured")

func (r *Runtime) timeout() time.Duration {
	if r.Timeout == 0 {
		return 10 * time.Second
	}
	return r.Timeout
}

// Available reports whether runtime commands can be sent.
func (r *Runtime) Available() bool { return r.MasterSocket != "" || r.StatsSocket != "" }

// CanReload reports whether reloads can be triggered.
func (r *Runtime) CanReload() bool { return r.MasterSocket != "" || r.ReloadCmd != "" }

func dial(addr string, to time.Duration) (net.Conn, error) {
	network := "unix"
	a := addr
	switch {
	case strings.HasPrefix(addr, "unix@"):
		a = strings.TrimPrefix(addr, "unix@")
	case strings.HasPrefix(addr, "tcp@"):
		network, a = "tcp", strings.TrimPrefix(addr, "tcp@")
	case !strings.HasPrefix(addr, "/") && strings.Contains(addr, ":"):
		network = "tcp"
	}
	return net.DialTimeout(network, a, to)
}

// send writes one command and reads the full answer.
func send(addr, cmd string, to time.Duration) (string, error) {
	c, err := dial(addr, to)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(to))
	if _, err := io.WriteString(c, cmd+"\n"); err != nil {
		return "", err
	}
	b, err := io.ReadAll(c)
	if err != nil && !errors.Is(err, io.EOF) {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() && len(b) > 0 {
			return string(b), nil
		}
		return string(b), err
	}
	return string(b), nil
}

// Exec runs a runtime command on the (current) worker.
func (r *Runtime) Exec(cmd string) (string, error) {
	if strings.ContainsAny(cmd, "\n;") {
		return "", fmt.Errorf("invalid runtime command")
	}
	switch {
	case r.StatsSocket != "":
		return send(r.StatsSocket, cmd, r.timeout())
	case r.MasterSocket != "":
		// The master CLI keeps the connection open after commands routed to
		// a worker; a trailing "quit" makes it close right after the answer.
		out, err := send(r.MasterSocket, "@1 "+cmd+"; quit", r.timeout())
		return strings.TrimLeft(out, "\n"), err
	}
	return "", ErrNoRuntime
}

// Info is a subset of "show info".
type Info struct {
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	UptimeSec int64  `json:"uptimeSec"`
	Pid       string `json:"pid"`
	CurrConns string `json:"currConns"`
	MaxConn   string `json:"maxConn"`
	NodeName  string `json:"node"`
}

// ShowInfo returns basic process information.
func (r *Runtime) ShowInfo() (*Info, error) {
	out, err := r.Exec("show info")
	if err != nil {
		return nil, err
	}
	kv := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	if kv["Version"] == "" {
		return nil, fmt.Errorf("unexpected answer from runtime API: %s", firstLine(out))
	}
	up, _ := strconv.ParseInt(kv["Uptime_sec"], 10, 64)
	return &Info{Version: kv["Version"], Uptime: kv["Uptime"], UptimeSec: up, Pid: kv["Pid"],
		CurrConns: kv["CurrConns"], MaxConn: kv["Maxconn"], NodeName: kv["node"]}, nil
}

// Stat is one row of "show stat".
type Stat struct {
	Proxy      string `json:"proxy"`
	Name       string `json:"name"` // FRONTEND, BACKEND or the server name
	Type       string `json:"type"` // frontend, backend, server, listener
	Status     string `json:"status"`
	CheckState string `json:"checkStatus,omitempty"`
	CheckDesc  string `json:"checkDesc,omitempty"`
	LastChange int64  `json:"lastChange"`
	Sessions   int64  `json:"sessions"`
	Rate       int64  `json:"rate"`
	Total      int64  `json:"total"`
	BytesIn    int64  `json:"bytesIn"`
	BytesOut   int64  `json:"bytesOut"`
	Weight     int64  `json:"weight"`
	Errors5xx  int64  `json:"http5xx"`
	Active     int64  `json:"active"`
	Backup     int64  `json:"backup"`
	Addr       string `json:"addr,omitempty"`
}

// ShowStat returns statistics for all proxies and servers.
func (r *Runtime) ShowStat() ([]Stat, error) {
	out, err := r.Exec("show stat")
	if err != nil {
		return nil, err
	}
	return ParseStatCSV(out)
}

// ParseStatCSV parses the CSV output of "show stat" or the stats page ";csv".
func ParseStatCSV(out string) ([]Stat, error) {
	out = strings.TrimPrefix(strings.TrimSpace(out), "# ")
	if out == "" {
		return nil, fmt.Errorf("empty statistics")
	}
	rd := csv.NewReader(strings.NewReader(out))
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cannot parse statistics: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty statistics")
	}
	idx := map[string]int{}
	for k, h := range rows[0] {
		idx[strings.TrimSpace(h)] = k
	}
	if _, ok := idx["pxname"]; !ok {
		return nil, fmt.Errorf("unexpected answer from runtime API: %s", firstLine(out))
	}
	get := func(row []string, k string) string {
		if i, ok := idx[k]; ok && i < len(row) {
			return row[i]
		}
		return ""
	}
	num := func(row []string, k string) int64 {
		n, _ := strconv.ParseInt(get(row, k), 10, 64)
		return n
	}
	types := map[string]string{"0": "frontend", "1": "backend", "2": "server", "3": "listener"}
	var stats []Stat
	for _, row := range rows[1:] {
		if len(row) < 2 {
			continue
		}
		stats = append(stats, Stat{
			Proxy: get(row, "pxname"), Name: get(row, "svname"), Type: types[get(row, "type")],
			Status: get(row, "status"), CheckState: get(row, "check_status"), CheckDesc: get(row, "check_desc"),
			LastChange: num(row, "lastchg"), Sessions: num(row, "scur"), Rate: num(row, "rate"),
			Total: num(row, "stot"), BytesIn: num(row, "bin"), BytesOut: num(row, "bout"),
			Weight: num(row, "weight"), Errors5xx: num(row, "hrsp_5xx"), Active: num(row, "act"),
			Backup: num(row, "bck"), Addr: get(row, "addr"),
		})
	}
	return stats, nil
}

// SetServerState switches a server to ready, drain or maint at runtime.
func (r *Runtime) SetServerState(backend, server, state string) error {
	switch state {
	case "ready", "drain", "maint":
	default:
		return fmt.Errorf("invalid state %q", state)
	}
	if !validRuntimeName(backend) || !validRuntimeName(server) {
		return fmt.Errorf("invalid backend or server name")
	}
	out, err := r.Exec(fmt.Sprintf("set server %s/%s state %s", backend, server, state))
	if err != nil {
		return err
	}
	if s := strings.TrimSpace(out); s != "" {
		return fmt.Errorf("%s", s)
	}
	return nil
}

func validRuntimeName(s string) bool {
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

// Reload asks HAProxy to reload its configuration.
func (r *Runtime) Reload(ctx context.Context) (string, error) {
	if r.MasterSocket != "" {
		out, err := send(r.MasterSocket, "reload", 60*time.Second)
		if err != nil {
			return out, fmt.Errorf("reload via master socket failed: %w", err)
		}
		// HAProxy >= 2.7 answers "Success=0|1" followed by the startup logs.
		if strings.Contains(out, "Success=0") {
			return out, fmt.Errorf("haproxy reported a failed reload")
		}
		return strings.TrimSpace(out), nil
	}
	if r.ReloadCmd != "" {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "/bin/sh", "-c", r.ReloadCmd).CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("reload command failed: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("no reload method configured")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if k := strings.IndexByte(s, '\n'); k >= 0 {
		return s[:k]
	}
	return s
}
