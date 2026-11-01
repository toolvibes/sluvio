import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api, type HistoryEntry } from "../api";
import { useT, type Key } from "../i18n";
import { useApp } from "../state";
import { services, serverState } from "../derive";
import { Badge, Empty, Icon, Spinner, StatusBadge } from "../components/ui";
import { fmtCompact, fmtRate, rowKey, sumRateSeries, trafficCounter } from "../stats";

export default function Dashboard() {
  const { t, fmtTime, lang } = useT();
  const { config, status, system, samples } = useApp();
  const reqRate = useMemo(() => {
    const keys = (status?.stats ?? []).filter((s) => s.type === "frontend").map(rowKey);
    return sumRateSeries(samples.slice(-2), keys, trafficCounter)[0]?.v ?? null;
  }, [samples, status]);
  const [recent, setRecent] = useState<HistoryEntry[] | null>(null);
  useEffect(() => {
    api
      .history(5)
      .then((r) => setRecent(r.entries))
      .catch(() => setRecent([]));
  }, [config?.version]);

  const svc = useMemo(() => (config ? services(config.model, status) : []), [config, status]);
  if (!config) return <Spinner label={t("common.loading")} />;
  const active = svc.filter((s) => s.backend.enabled);
  const disabled = svc.filter((s) => !s.backend.enabled);
  const live = status?.available;
  let up = 0,
    down = 0,
    maint = 0;
  for (const s of active) {
    up += s.up;
    down += s.down;
    maint += s.maint;
  }
  const problems: { key: string; backend: string; text: string; status?: string }[] = [];
  if (live) {
    for (const s of active) {
      for (const srv of s.backend.servers) {
        if (!srv.enabled) continue;
        const st = serverState(status?.stats, s.backend.name, srv.name);
        const stat = status?.stats.find((x) => x.proxy === s.backend.name && x.name === srv.name);
        if (stat?.checkStatus?.includes("INI")) continue; // first check still pending
        if (st && (st.toUpperCase().startsWith("DOWN") || st.toUpperCase().startsWith("UP "))) {
          problems.push({
            key: s.backend.name + "/" + srv.name,
            backend: s.backend.name,
            text: `${s.backend.name} / ${srv.name}` + (stat?.checkDesc ? ` – ${stat.checkDesc}` : ""),
            status: st,
          });
        }
      }
      if (s.total > 0 && s.up === 0) {
        problems.push({ key: s.backend.name, backend: s.backend.name, text: t("dash.noHealthy", { name: s.backend.name }) });
      }
    }
  }
  const warnings = (system?.checks ?? []).filter((c) => !c.ok);

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>{t("dash.title")}</h1>
          <p className="muted mono small">{config.path}</p>
        </div>
        <Link to="/services/new" className="btn btn-primary">
          <Icon name="plus" /> {t("services.new")}
        </Link>
      </div>

      <div className="tiles">
        <div className="tile">
          <div className="tile-label">HAProxy</div>
          <div className="tile-value">{live ? status?.info?.version.split("-")[0] : "–"}</div>
          <div className="tile-sub muted">{live ? t("dash.uptime", { v: status?.info?.uptime ?? "" }) : t("app.noLiveStatus")}</div>
        </div>
        <Link to="/stats" className="tile tile-link">
          <div className="tile-label">{t("dash.requests")}</div>
          <div className="tile-value">{live ? fmtRate(reqRate, lang) : "–"}</div>
          <div className="tile-sub muted">
            {live ? t("dash.requestsSub", { n: fmtCompact(status?.info?.cumReq, lang), c: status?.info?.currConns ?? "0" }) : t("app.noLiveStatus")}
          </div>
        </Link>
        <div className="tile">
          <div className="tile-label">{t("dash.services")}</div>
          <div className="tile-value">{active.length}</div>
          <div className="tile-sub muted">{t("dash.servicesDisabled", { n: disabled.length })}</div>
        </div>
        <div className="tile">
          <div className="tile-label">{t("dash.servers")}</div>
          <div className="tile-value">
            {live ? (
              <span className="tile-split">
                <span className="tone-text-ok">{up}</span>
                <span className="muted">/</span>
                <span className={down ? "tone-text-error" : "muted"}>{down}</span>
                <span className="muted">/</span>
                <span className={maint ? "tone-text-maint" : "muted"}>{maint}</span>
              </span>
            ) : (
              active.reduce((n, s) => n + s.total, 0)
            )}
          </div>
          <div className="tile-sub muted">{live ? t("dash.serversLegend") : t("dash.serversConfigured")}</div>
        </div>
      </div>

      <div className="grid-2">
        <section className="card">
          <h2 className="card-title">{t("dash.problems")}</h2>
          {!live ? (
            <Empty>
              {t("dash.noLive")} <Link to="/setup">{t("nav.setup")}</Link>
            </Empty>
          ) : problems.length === 0 ? (
            <div className="all-good">
              <Icon name="check" /> {t("dash.allGood")}
            </div>
          ) : (
            <ul className="list">
              {problems.map((p) => (
                <li key={p.key}>
                  {p.status ? <StatusBadge status={p.status} /> : <Badge tone="error">{t("dash.noHealthyShort")}</Badge>}
                  <Link to={`/services?q=${encodeURIComponent(p.backend)}`} className="mono">
                    {p.text}
                  </Link>
                </li>
              ))}
            </ul>
          )}
          {warnings.length > 0 && (
            <>
              <h3 className="card-subtitle">{t("dash.setupWarnings")}</h3>
              <ul className="list">
                {warnings.map((w) => (
                  <li key={w.id}>
                    <Badge tone={w.level === "error" ? "error" : "warn"}>{w.level === "error" ? t("setup.error") : t("setup.warn")}</Badge>
                    <Link to="/setup">{t(("check." + w.id) as Key, w.params)}</Link>
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>

        <section className="card">
          <h2 className="card-title">{t("dash.recent")}</h2>
          {recent === null ? (
            <Spinner />
          ) : recent.length === 0 ? (
            <Empty>{t("history.empty")}</Empty>
          ) : (
            <ul className="list">
              {recent.map((e) => (
                <li key={e.id}>
                  <Badge tone={kindTone(e.kind)}>{t(("history.kind." + e.kind) as Key)}</Badge>
                  <Link to={`/history/${e.id}`}>{e.message || e.ops?.join(", ") || "#" + e.id}</Link>
                  <span className="muted small nowrap">{fmtTime(e.time)}</span>
                </li>
              ))}
            </ul>
          )}
          <Link to="/history" className="btn-link">
            {t("dash.allHistory")} <Icon name="chevron" />
          </Link>
        </section>
      </div>
    </div>
  );
}

export function kindTone(kind: string) {
  switch (kind) {
    case "change":
      return "accent" as const;
    case "external":
      return "warn" as const;
    case "rollback":
      return "maint" as const;
    case "raw":
      return "accent" as const;
    default:
      return "neutral" as const;
  }
}
