<p align="center">
  <img src="docs/logo.svg" width="72" alt="" />
</p>

<h1 align="center">SluiceOps</h1>

<p align="center">
  A web UI to manage HAProxy — that keeps your <code>haproxy.cfg</code> exactly the way you wrote it.<br/>
  <a href="README.de.md">Deutsche Version</a>
</p>

---

SluiceOps lets you add services, switch routes and servers on and off, watch the live status and edit the configuration of an existing HAProxy from the browser. It is free software (AGPL-3.0), has no paid tier and no feature gates.

> [!WARNING]
> **Early version (0.x).** SluiceOps is new and has so far only been tested on a few setups. Before you let it manage a production HAProxy:
>
> - try it with a copy of your configuration first, or start it with `-read-only` and look around,
> - check the diff in the review dialog before every change,
> - keep your own backup of `haproxy.cfg` (SluiceOps keeps a history, but do not rely on it alone).
>
> Bug reports and feedback are very welcome — please [open an issue](https://github.com/toolvibes/sluiceops/issues).

## Why another HAProxy UI?

Most tools parse `haproxy.cfg` into a data model and write a freshly generated file back. Comments move to the top of their section or disappear, sections get reordered, `inter 5s` becomes `inter 5000`. For a configuration that is documented and maintained by hand, that is not acceptable.

SluiceOps works differently:

- **Line-preserving editor.** Every change touches only the lines it is about. Comments, blank lines, alignment and order stay byte for byte. Disabling and re-enabling something gives you back the identical file.
- **Your config stays the source of truth.** There is no database and no import step. Point SluiceOps at an existing `haproxy.cfg` and it works. You can keep editing the file by hand; SluiceOps notices and records such changes. Uninstalling it leaves a normal HAProxy config behind.
- **"Disabled" means commented out** — just like you would do it by hand. SluiceOps recognises commented-out `use_backend` rules, ACLs, servers and whole `backend` blocks as disabled configuration. Enabling a prepared service uncomments its ACLs, its rule and its backend block in one step.
- **Every change is reviewed and validated.** You see the diff first, `haproxy -c` checks the result, only then the file is written and HAProxy is reloaded gracefully. Every version is kept and can be restored.
- **Live where possible.** Putting a server into maintenance or drain happens through the runtime API without a reload, and is also persisted in the file (`disabled`) so that it survives the next reload.

## Features

- Services overview: every backend with the hosts/paths that route to it, servers with live health status, enable/disable switches
- New-service wizard (API, web UI, WebSocket, TCP presets): backend, servers, health check and host/path route in one change
- Routing view per frontend in the exact rule order (first match wins), add/edit/disable/delete routes, default backend
- Live statistics per frontend, backend and server: requests/s with history, sessions, HTTP response classes, errors, response times, traffic
- Server management: add, edit, maintenance, drain, delete
- Backend settings: mode, balance algorithm, health check, timeouts; raw editing of single sections
- Full config editor with validation and error line markers
- History of all versions (including changes made outside SluiceOps), diffs, one-click rollback
- Setup checks (write access, validator, runtime socket, reload method, version match)
- German and English UI, light and dark theme
- Single static binary or Docker image, no database

## Quick start with Docker Compose

Assuming your config lives in `./haproxy/haproxy.cfg`:

```yaml
services:
  haproxy:
    image: haproxy:2.9-alpine
    command: ["haproxy", "-W", "-db", "-f", "/usr/local/etc/haproxy/haproxy.cfg", "-S", "/var/lib/haproxy/master.sock"]
    volumes:
      - ./haproxy:/usr/local/etc/haproxy        # directory, not the single file
      - haproxy-run:/var/lib/haproxy             # shared master socket
    ports: ["80:80"]

  sluiceops:
    image: ghcr.io/toolvibes/sluiceops:haproxy2.9 # same HAProxy version as above
    environment:
      SLUICEOPS_MASTER_SOCKET: /var/lib/haproxy/master.sock
      SLUICEOPS_ADMIN_PASSWORD: change-me-please
    volumes:
      - ./haproxy:/usr/local/etc/haproxy
      - haproxy-run:/var/lib/haproxy
      - sluiceops-data:/var/lib/sluiceops
    ports: ["127.0.0.1:8484:8484"]

volumes:
  haproxy-run:
  sluiceops-data:
```

Both containers run as uid 99 (`haproxy` user of the official image). Give it write access to the config directory once:

```bash
sudo chown -R 99:99 ./haproxy
```

Then open http://127.0.0.1:8484 and log in as `admin`. A complete, commented example is in [`deploy/docker-compose.example.yml`](deploy/docker-compose.example.yml).

**Why these changes to a typical HAProxy compose file?**

| Change | Reason |
|---|---|
| Mount the directory, not the file | Single-file bind mounts break when a file is replaced; SluiceOps writes in place, but a directory mount is robust for all editors. |
| `-S /var/lib/haproxy/master.sock` | The master socket provides live status, maintenance/drain without reload and graceful reloads — without giving anything access to the Docker socket. |
| Same HAProxy version in both images | Validation runs `haproxy -c` inside the SluiceOps container. Images are published per HAProxy version: `haproxy2.8`, `haproxy2.9`, `haproxy3.0`, `haproxy3.2`. |

## Running the binary

```bash
sluiceops \
  -config /etc/haproxy/haproxy.cfg \
  -master-socket /run/haproxy/master.sock \
  -data-dir /var/lib/sluiceops
```

Without a master socket you can use a stats socket (`stats socket /run/haproxy/admin.sock mode 660 level admin` in `global`) together with a reload command:

```bash
sluiceops -config /etc/haproxy/haproxy.cfg \
  -stats-socket /run/haproxy/admin.sock \
  -reload-cmd "systemctl reload haproxy"
```

### Options

Every flag can also be set as environment variable.

| Flag | Environment | Default | Description |
|---|---|---|---|
| `-config` | `SLUICEOPS_CONFIG` | `/usr/local/etc/haproxy/haproxy.cfg` | HAProxy configuration file |
| `-listen` | `SLUICEOPS_LISTEN` | `:8484` | Address of the web UI |
| `-haproxy-bin` | `SLUICEOPS_HAPROXY_BIN` | `haproxy` | Binary for `haproxy -c`; `none` disables validation |
| `-master-socket` | `SLUICEOPS_MASTER_SOCKET` | – | Master CLI socket (`haproxy -W -S …`): status, runtime changes, reloads |
| `-stats-socket` | `SLUICEOPS_STATS_SOCKET` | – | Stats socket with `level admin` (alternative) |
| `-reload-cmd` | `SLUICEOPS_RELOAD_CMD` | – | Shell command for reloads when no master socket is used |
| `-data-dir` | `SLUICEOPS_DATA_DIR` | `/var/lib/sluiceops` | History and users |
| `-history-keep` | `SLUICEOPS_HISTORY_KEEP` | `500` | Number of versions to keep (`0` = all) |
| `-read-only` | `SLUICEOPS_READ_ONLY` | `false` | Never write the configuration |
| `-auth` | `SLUICEOPS_AUTH` | `password` | `password` or `none` (only behind an authenticating proxy) |
| `-admin-user` | `SLUICEOPS_ADMIN_USER` | `admin` | Name of the admin user |
| – | `SLUICEOPS_ADMIN_PASSWORD` | – | Sets the admin password at start (min. 8 characters). If no user exists and it is unset, a random password is printed once to the log. |

## How a change is applied

1. The current file is read and its version (content hash) compared with the one your browser has. If someone else changed the file in the meantime, you get a conflict instead of overwriting their work.
2. The requested operations are applied to a copy, line by line.
3. You see the diff. `haproxy -c` validates the complete result.
4. The file is written in place, the previous version is in the history.
5. HAProxy is reloaded gracefully through the master socket — or, for server maintenance toggles, the change is applied live via the runtime API without any reload.

## Security

Whoever can use SluiceOps controls all traffic that passes your HAProxy. Run it in your internal network or behind your reverse proxy with additional access control; do not expose it to the internet. Sessions use `HttpOnly`/`SameSite=Strict` cookies, mutating requests require a custom header (CSRF protection), failed logins are rate limited. See [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## Limitations

- The structured forms cover the common cases (services, servers, host/path routing, health checks, timeouts). Everything else stays untouched and can be edited in the section or full config editor.
- Routes with complex conditions (`||`, negations, other fetches) are shown and can be switched on/off, but must be edited as text.
- One HAProxy instance per SluiceOps instance; configuration split over several files (`-f dir/`) is not supported yet.

## Development

```bash
make ui      # build the web UI (Node 22)
make build   # build the binary with the embedded UI (Go 1.26)
make test    # unit tests; tests against a real HAProxy run if "haproxy" is installed
make run     # read-only demo against testdata/realworld.cfg
```

For UI development run `npm run dev` in `web/` (proxies `/api` to `127.0.0.1:8484`).

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

SluiceOps is licensed under the [GNU Affero General Public License v3.0](LICENSE). You may use, modify and redistribute it. If you offer a modified version to others over a network, you must make your source code available to them as well.

HAProxy is a trademark of HAProxy Technologies LLC. SluiceOps is an independent project and not affiliated with HAProxy Technologies.
