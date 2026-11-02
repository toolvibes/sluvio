import { useEffect } from "react";
import { useT, type Key } from "../i18n";
import { useApp } from "../state";
import { Badge, CopyBlock, Icon, Spinner } from "../components/ui";

const composeSnippet = `services:
  haproxy:
    image: haproxy:2.9-alpine
    restart: unless-stopped
    # -W: master-worker, -S: master socket for status + reload
    command: ["haproxy", "-W", "-db", "-f", "/usr/local/etc/haproxy/haproxy.cfg",
              "-S", "/var/lib/haproxy/master.sock"]
    volumes:
      - ./haproxy:/usr/local/etc/haproxy      # directory, not a single file
      - haproxy-run:/var/lib/haproxy
    ports:
      - "80:80"

  sluvio:
    image: ghcr.io/toolvibes/sluvio:latest
    restart: unless-stopped
    environment:
      SLUVIO_CONFIG: /usr/local/etc/haproxy/haproxy.cfg
      SLUVIO_MASTER_SOCKET: /var/lib/haproxy/master.sock
      SLUVIO_ADMIN_PASSWORD: \${SLUVIO_ADMIN_PASSWORD}
    volumes:
      - ./haproxy:/usr/local/etc/haproxy
      - haproxy-run:/var/lib/haproxy
      - sluvio-data:/var/lib/sluvio
    ports:
      - "127.0.0.1:8484:8484"

volumes:
  haproxy-run:
  sluvio-data:`;

const socketSnippet = `global
    stats socket /var/lib/haproxy/admin.sock mode 660 level admin`;

const chownSnippet = `sudo chown -R 99:99 ./haproxy`;

export default function Setup() {
  const { t } = useT();
  const { system, refreshSystem } = useApp();
  useEffect(() => {
    void refreshSystem();
  }, [refreshSystem]);
  if (!system) return <Spinner label={t("common.loading")} />;
  return (
    <div className="page page-narrow">
      <div className="page-head">
        <div>
          <h1>{t("setup.title")}</h1>
          <p className="muted">{t("setup.subtitle")}</p>
        </div>
        <button type="button" className="btn" onClick={() => void refreshSystem()}>
          <Icon name="restore" /> {t("setup.recheck")}
        </button>
      </div>
      <section className="card">
        <h2 className="card-title">{t("setup.checks")}</h2>
        <ul className="checks">
          {system.checks.map((c) => (
            <li key={c.id} className={"check check-" + c.level}>
              <span className="check-icon" aria-hidden="true">
                <Icon name={c.level === "ok" ? "check" : c.level === "warn" ? "warn" : "error"} />
              </span>
              <div className="check-body">
                <div className="check-title">
                  <strong>{t(("check." + c.id) as Key, c.params)}</strong>
                  <Badge tone={c.level === "ok" ? "ok" : c.level === "warn" ? "warn" : "error"}>
                    {c.level === "ok" ? t("setup.ok") : c.level === "warn" ? t("setup.warn") : t("setup.error")}
                  </Badge>
                </div>
                {c.detail && <div className="muted small mono break">{c.detail}</div>}
                {!c.ok && <p className="check-hint">{t(("check." + c.id + ".hint") as Key, c.params)}</p>}
                {!c.ok && c.id === "config_writable" && <CopyBlock text={chownSnippet} />}
                {!c.ok && (c.id === "stats_socket_hint" || c.id === "runtime") && <CopyBlock text={socketSnippet} label={t("setup.socketAlt")} />}
              </div>
            </li>
          ))}
        </ul>
      </section>
      <section className="card">
        <h2 className="card-title">{t("setup.settings")}</h2>
        <table className="kv">
          <tbody>
            {Object.entries(system.settings).map(([k, v]) => (
              <tr key={k}>
                <th scope="row">{t(("setting." + k) as Key)}</th>
                <td className="mono break">{v}</td>
              </tr>
            ))}
            <tr>
              <th scope="row">{t("setting.version")}</th>
              <td className="mono">{system.version}</td>
            </tr>
          </tbody>
        </table>
        <p className="muted small">{t("setup.settingsHint")}</p>
      </section>
      <section className="card">
        <h2 className="card-title">{t("setup.docker")}</h2>
        <p>{t("setup.dockerIntro")}</p>
        <ul className="bullets">
          <li>{t("setup.dockerPoint1")}</li>
          <li>{t("setup.dockerPoint2")}</li>
          <li>{t("setup.dockerPoint3")}</li>
        </ul>
        <CopyBlock text={composeSnippet} label="docker-compose.yml" />
      </section>
    </div>
  );
}
