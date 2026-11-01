// SluiceOps is a web UI to manage HAProxy configurations without losing the
// structure, comments and order of hand written files.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/toolvibes/sluiceops/internal/app"
	"github.com/toolvibes/sluiceops/internal/haproxy"
	"github.com/toolvibes/sluiceops/internal/history"
	"github.com/toolvibes/sluiceops/internal/server"
	"github.com/toolvibes/sluiceops/web"
)

var version = "dev"

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

type options struct {
	listen, configPath, haproxyBin       string
	masterSocket, statsSocket, reloadCmd string
	dataDir, authMode, adminUser         string
	keep                                 int
	readOnly                             bool
	sessionTTL                           time.Duration
}

func main() {
	var o options
	flag.StringVar(&o.listen, "listen", env("SLUICEOPS_LISTEN", ":8484"), "address of the web UI")
	flag.StringVar(&o.configPath, "config", env("SLUICEOPS_CONFIG", "/usr/local/etc/haproxy/haproxy.cfg"), "path of haproxy.cfg")
	flag.StringVar(&o.haproxyBin, "haproxy-bin", env("SLUICEOPS_HAPROXY_BIN", "haproxy"), "haproxy binary used for validation (\"none\" disables validation)")
	flag.StringVar(&o.masterSocket, "master-socket", env("SLUICEOPS_MASTER_SOCKET", ""), "HAProxy master CLI socket (haproxy -W -S <path>); enables status and reloads")
	flag.StringVar(&o.statsSocket, "stats-socket", env("SLUICEOPS_STATS_SOCKET", ""), "HAProxy stats socket with \"level admin\" (alternative to -master-socket)")
	flag.StringVar(&o.reloadCmd, "reload-cmd", env("SLUICEOPS_RELOAD_CMD", ""), "shell command to reload HAProxy, if no master socket is used")
	flag.StringVar(&o.dataDir, "data-dir", env("SLUICEOPS_DATA_DIR", "/var/lib/sluiceops"), "directory for history and users")
	flag.IntVar(&o.keep, "history-keep", envInt("SLUICEOPS_HISTORY_KEEP", 500), "number of config versions to keep (0 = all)")
	flag.BoolVar(&o.readOnly, "read-only", envBool("SLUICEOPS_READ_ONLY", false), "never write the configuration")
	flag.StringVar(&o.authMode, "auth", env("SLUICEOPS_AUTH", "password"), "authentication: password or none (only behind an authenticating proxy!)")
	flag.StringVar(&o.adminUser, "admin-user", env("SLUICEOPS_ADMIN_USER", "admin"), "name of the admin user")
	flag.DurationVar(&o.sessionTTL, "session-ttl", 12*time.Hour, "idle time after which a session expires")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("sluiceops", version)
		return
	}
	server.Version = version
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log, o); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, o options) error {
	if _, err := os.Stat(o.configPath); err != nil {
		return fmt.Errorf("config file: %w", err)
	}
	if abs, err := filepath.Abs(o.configPath); err == nil {
		o.configPath = abs
	}
	bin := o.haproxyBin
	if bin == "none" || bin == "" {
		bin = ""
		log.Warn("validation disabled: changes are written without haproxy -c")
	} else if p, err := exec.LookPath(bin); err == nil {
		bin = p
	} else {
		return fmt.Errorf("haproxy binary %q not found (needed for validation; use -haproxy-bin none to disable): %w", bin, err)
	}
	if err := os.MkdirAll(o.dataDir, 0o700); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	hist, err := history.Open(filepath.Join(o.dataDir, "history"), o.keep)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	rt := &haproxy.Runtime{MasterSocket: o.masterSocket, StatsSocket: o.statsSocket, ReloadCmd: o.reloadCmd}
	a := &app.App{
		ConfigPath: o.configPath,
		ReadOnly:   o.readOnly,
		Validator:  &haproxy.Validator{Binary: bin, ConfigPath: o.configPath},
		Runtime:    rt,
		History:    hist,
	}
	if _, err := a.Load(); err != nil {
		return err
	}
	auth, generated, err := server.NewAuth(o.dataDir, o.adminUser, os.Getenv("SLUICEOPS_ADMIN_PASSWORD"), o.sessionTTL)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	switch o.authMode {
	case "password":
	case "none":
		auth.Disabled = true
		log.Warn("authentication disabled – only run SluiceOps like this behind an authenticating proxy")
	default:
		return fmt.Errorf("unknown -auth %q", o.authMode)
	}
	if generated != "" {
		log.Warn("created admin user with a generated password – set SLUICEOPS_ADMIN_PASSWORD to choose your own", "user", o.adminUser, "password", generated)
	}
	if !rt.Available() {
		log.Warn("no runtime socket configured: live status and server toggles without reload are unavailable")
	}
	ui, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}
	dash := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "–"
		}
		return s
	}
	srv := &server.Server{App: a, Auth: auth, UI: ui, Log: log, Settings: map[string]string{
		"config":       o.configPath,
		"haproxyBin":   dash(bin),
		"masterSocket": dash(o.masterSocket),
		"statsSocket":  dash(o.statsSocket),
		"reloadCmd":    dash(o.reloadCmd),
		"dataDir":      o.dataDir,
		"auth":         o.authMode,
	}}
	hs := &http.Server{Addr: o.listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	log.Info("SluiceOps started", "version", version, "listen", o.listen, "config", o.configPath, "readOnly", o.readOnly)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
