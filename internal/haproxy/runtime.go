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
	Version     string `json:"version"`
	Uptime      string `json:"uptime"`
	UptimeSec   int64  `json:"uptimeSec"`
	Pid         string `json:"pid"`
	CurrConns   string `json:"currConns"`
	MaxConn     string `json:"maxConn"`
	NodeName    string `json:"node"`
	Threads     int64  `json:"threads"`
	CumConns    int64  `json:"cumConns"`
	CumReq      int64  `json:"cumReq"`
	ConnRate    int64  `json:"connRate"`
	MaxConnRate int64  `json:"maxConnRate"`
	SessRate    int64  `json:"sessRate"`
	MaxSessRate int64  `json:"maxSessRate"`
	IdlePct     int64  `json:"idlePct"`
	RunQueue    int64  `json:"runQueue"`
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
	n := func(k string) int64 {
		v, _ := strconv.ParseInt(kv[k], 10, 64)
		return v
	}
	return &Info{Version: kv["Version"], Uptime: kv["Uptime"], UptimeSec: up, Pid: kv["Pid"],
		CurrConns: kv["CurrConns"], MaxConn: kv["Maxconn"], NodeName: kv["node"],
		Threads: n("Nbthread"), CumConns: n("CumConns"), CumReq: n("CumReq"),
		ConnRate: n("ConnRate"), MaxConnRate: n("MaxConnRate"), SessRate: n("SessRate"),
		MaxSessRate: n("MaxSessRate"), IdlePct: n("Idle_pct"), RunQueue: n("Run_queue")}, nil
}

// Stat is one row of "show stat". Counters are cumulative since the last
// (re)start of the worker, rates are per second over the last second, times
// are averages in milliseconds over the last 1024 requests.
type Stat struct {
	Proxy      string `json:"proxy"`
	Name       string `json:"name"` // FRONTEND, BACKEND or the server name
	Type       string `json:"type"` // frontend, backend, server, listener
	Status     string `json:"status"`
	Mode       string `json:"mode,omitempty"`
	Algo       string `json:"algo,omitempty"`
	CheckState string `json:"checkStatus,omitempty"`
	CheckDesc  string `json:"checkDesc,omitempty"`
	CheckCode  int64  `json:"checkCode,omitempty"`
	CheckTime  int64  `json:"checkDuration,omitempty"`
	LastChange int64  `json:"lastChange"`
	LastSess   int64  `json:"lastSess"` // seconds since last session, -1 = never
	Downtime   int64  `json:"downtime"`
	ChkFail    int64  `json:"chkFail"`
	ChkDown    int64  `json:"chkDown"`
	Addr       string `json:"addr,omitempty"`
	Weight     int64  `json:"weight"`
	Active     int64  `json:"active"`
	Backup     int64  `json:"backup"`

	Queue        int64 `json:"queue"`
	QueueMax     int64 `json:"queueMax"`
	Sessions     int64 `json:"sessions"` // currently open
	SessionsMax  int64 `json:"sessionsMax"`
	SessionLimit int64 `json:"sessionLimit"`
	Total        int64 `json:"total"` // sessions since start
	LbTotal      int64 `json:"lbTotal"`
	Rate         int64 `json:"rate"` // sessions per second
	RateMax      int64 `json:"rateMax"`
	ReqRate      int64 `json:"reqRate"` // HTTP requests per second (frontends)
	ReqRateMax   int64 `json:"reqRateMax"`
	ReqTotal     int64 `json:"reqTotal"`
	BytesIn      int64 `json:"bytesIn"`
	BytesOut     int64 `json:"bytesOut"`

	Http1xx   int64 `json:"http1xx"`
	Http2xx   int64 `json:"http2xx"`
	Http3xx   int64 `json:"http3xx"`
	Http4xx   int64 `json:"http4xx"`
	Errors5xx int64 `json:"http5xx"`
	HttpOther int64 `json:"httpOther"`

	DeniedReq  int64 `json:"deniedReq"`
	DeniedResp int64 `json:"deniedResp"`
	ErrReq     int64 `json:"errReq"`
	ErrConn    int64 `json:"errConn"`
	ErrResp    int64 `json:"errResp"`
	Retries    int64 `json:"retries"`
	Redispatch int64 `json:"redispatch"`
	CliAbort   int64 `json:"cliAbort"`
	SrvAbort   int64 `json:"srvAbort"`

	QTime int64 `json:"qtime"`
	CTime int64 `json:"ctime"`
	RTime int64 `json:"rtime"`
	TTime int64 `json:"ttime"`
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
			Status: get(row, "status"), Mode: get(row, "mode"), Algo: get(row, "algo"),
			CheckState: get(row, "check_status"), CheckDesc: get(row, "check_desc"),
			CheckCode: num(row, "check_code"), CheckTime: num(row, "check_duration"),
			LastChange: num(row, "lastchg"), LastSess: num(row, "lastsess"), Downtime: num(row, "downtime"),
			ChkFail: num(row, "chkfail"), ChkDown: num(row, "chkdown"), Addr: get(row, "addr"),
			Weight: num(row, "weight"), Active: num(row, "act"), Backup: num(row, "bck"),
			Queue: num(row, "qcur"), QueueMax: num(row, "qmax"),
			Sessions: num(row, "scur"), SessionsMax: num(row, "smax"), SessionLimit: num(row, "slim"),
			Total: num(row, "stot"), LbTotal: num(row, "lbtot"), Rate: num(row, "rate"), RateMax: num(row, "rate_max"),
			ReqRate: num(row, "req_rate"), ReqRateMax: num(row, "req_rate_max"), ReqTotal: num(row, "req_tot"),
			BytesIn: num(row, "bin"), BytesOut: num(row, "bout"),
			Http1xx: num(row, "hrsp_1xx"), Http2xx: num(row, "hrsp_2xx"), Http3xx: num(row, "hrsp_3xx"),
			Http4xx: num(row, "hrsp_4xx"), Errors5xx: num(row, "hrsp_5xx"), HttpOther: num(row, "hrsp_other"),
			DeniedReq: num(row, "dreq"), DeniedResp: num(row, "dresp"), ErrReq: num(row, "ereq"),
			ErrConn: num(row, "econ"), ErrResp: num(row, "eresp"), Retries: num(row, "wretr"),
			Redispatch: num(row, "wredis"), CliAbort: num(row, "cli_abrt"), SrvAbort: num(row, "srv_abrt"),
			QTime: num(row, "qtime"), CTime: num(row, "ctime"), RTime: num(row, "rtime"), TTime: num(row, "ttime"),
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
