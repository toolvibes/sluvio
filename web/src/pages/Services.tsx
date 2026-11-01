import { useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, serverStatOf, specOf, type Backend, type Server, type Stat } from "../api";
import { useT } from "../i18n";
import { useApp } from "../state";
import { services, type ServiceView, type RouteRef } from "../derive";
import { Badge, Empty, Icon, Spinner, StatusBadge, Switch, useToast } from "../components/ui";
import { useReview } from "../components/review";
import { BackendDialog, DisableServiceDialog, RouteDialog, SectionDialog, ServerDialog } from "../components/dialogs";

type Filter = "all" | "active" | "disabled" | "problems";

type DialogState =
  | { kind: "server"; backend: Backend; server?: Server }
  | { kind: "settings"; backend: Backend }
  | { kind: "section"; backend: Backend }
  | { kind: "disable"; backend: Backend; defaultOf: string[] }
  | { kind: "route"; backend: Backend; route?: RouteRef }
  | null;

export default function Services() {
  const { t } = useT();
  const { config, status, readOnly } = useApp();
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const filter = (params.get("f") as Filter) || "all";
  const [dialog, setDialog] = useState<DialogState>(null);
  const all = useMemo(() => (config ? services(config.model, status) : []), [config, status]);
  if (!config) return <Spinner label={t("common.loading")} />;

  const needle = q.trim().toLowerCase();
  const shown = all.filter((s) => {
    if (filter === "active" && !s.backend.enabled) return false;
    if (filter === "disabled" && s.backend.enabled) return false;
    if (filter === "problems" && !s.problem) return false;
    if (!needle) return true;
    const hay = [
      s.backend.name,
      s.backend.description ?? "",
      ...s.backend.servers.map((x) => `${x.name} ${x.address}:${x.port ?? ""}`),
      ...s.routes.flatMap((r) => [...(r.rule?.hosts ?? []), ...(r.rule?.paths ?? []), r.frontend.name]),
    ]
      .join(" ")
      .toLowerCase();
    return hay.includes(needle);
  });
  const counts = {
    all: all.length,
    active: all.filter((s) => s.backend.enabled).length,
    disabled: all.filter((s) => !s.backend.enabled).length,
    problems: all.filter((s) => s.problem).length,
  };
  const setParam = (k: string, v: string) => {
    const p = new URLSearchParams(params);
    if (v) p.set(k, v);
    else p.delete(k);
    setParams(p, { replace: true });
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>{t("services.title")}</h1>
          <p className="muted">{t("services.subtitle")}</p>
        </div>
        {!readOnly && (
          <Link to="/services/new" className="btn btn-primary">
            <Icon name="plus" /> {t("services.new")}
          </Link>
        )}
      </div>
      <div className="toolbar">
        <div className="search">
          <Icon name="search" />
          <input
            type="search"
            value={q}
            placeholder={t("services.search")}
            aria-label={t("services.search")}
            onChange={(e) => setParam("q", e.target.value)}
          />
        </div>
        <div className="seg" role="group" aria-label={t("services.filter")}>
          {(["all", "active", "disabled", "problems"] as Filter[]).map((f) => (
            <button key={f} type="button" aria-pressed={filter === f} className={filter === f ? "active" : ""} onClick={() => setParam("f", f === "all" ? "" : f)}>
              {t(("services.filter." + f) as "services.filter.all")} <span className="count">{counts[f]}</span>
            </button>
          ))}
        </div>
      </div>
      {shown.length === 0 ? (
        <Empty>{t("services.none")}</Empty>
      ) : (
        <div className="service-list">
          {shown.map((s) => (
            <ServiceCard key={s.backend.name + s.backend.line} s={s} stats={status?.available ? status.stats : undefined} live={!!status?.available} onDialog={setDialog} />
          ))}
        </div>
      )}
      {dialog?.kind === "server" && (
        <ServerDialog backend={dialog.backend} server={dialog.server ? specOf(dialog.server) : undefined} onClose={() => setDialog(null)} />
      )}
      {dialog?.kind === "settings" && <BackendDialog backend={dialog.backend} onClose={() => setDialog(null)} />}
      {dialog?.kind === "section" && <SectionDialog kind={dialog.backend.kind} name={dialog.backend.name} onClose={() => setDialog(null)} />}
      {dialog?.kind === "disable" && <DisableServiceDialog backend={dialog.backend} defaultOf={dialog.defaultOf} onClose={() => setDialog(null)} />}
      {dialog?.kind === "route" && (
        <RouteDialog frontend={dialog.route?.frontend} rule={dialog.route?.rule} defaultBackend={dialog.backend.name} onClose={() => setDialog(null)} />
      )}
    </div>
  );
}

function ServiceCard({
  s,
  stats,
  live,
  onDialog,
}: {
  s: ServiceView;
  stats?: Stat[];
  live: boolean;
  onDialog: (d: DialogState) => void;
}) {
  const { t } = useT();
  const { readOnly } = useApp();
  const review = useReview();
  const b = s.backend;
  const [descOpen, setDescOpen] = useState(false);
  const desc = b.description?.trim();
  const descFirst = desc?.split("\n")[0];
  const defaultOf = s.routes.filter((r) => r.isDefault && r.enabled).map((r) => r.frontend.name);
  const toggle = async (on: boolean) => {
    if (!on) {
      onDialog({ kind: "disable", backend: b, defaultOf });
      return;
    }
    await review({ title: t("service.enableTitle", { name: b.name }), ops: [{ op: "service.setEnabled", backend: b.name, enabled: true }], compact: true });
  };
  return (
    <article className={"service" + (b.enabled ? "" : " is-disabled") + (s.problem ? " has-problem" : "")} aria-labelledby={"svc-" + b.name}>
      <header className="service-head">
        <div className="service-title">
          <h2 id={"svc-" + b.name} className="mono">
            {b.name}
          </h2>
          <div className="row-wrap">
            {b.enabled ? (
              live && s.total > 0 ? (
                <Badge tone={s.problem ? "error" : s.down > 0 ? "warn" : "ok"}>{t("service.healthy", { up: s.up, total: s.total })}</Badge>
              ) : (
                <Badge tone="ok">{t("common.enabled")}</Badge>
              )
            ) : (
              <Badge tone="neutral">{t("service.disabledCommented")}</Badge>
            )}
            {b.kind === "listen" && <Badge>listen</Badge>}
            {b.mode && <Badge>{b.mode}</Badge>}
            {b.balance && <Badge title={t("backend.balance")}>{b.balance}</Badge>}
            {b.healthCheck?.path && (
              <Badge title={t("backend.httpCheck")}>
                <span className="mono">
                  {b.healthCheck.method ?? "GET"} {b.healthCheck.path}
                </span>
              </Badge>
            )}
            {b.timeouts?.tunnel && <Badge title="timeout tunnel">WebSocket · {b.timeouts.tunnel}</Badge>}
            <span className="muted small">{t("common.lineN", { n: b.line + 1 })}</span>
          </div>
        </div>
        <div className="service-switch">
          <Switch checked={b.enabled} onChange={toggle} label={b.enabled ? t("service.disable", { name: b.name }) : t("service.enable", { name: b.name })} disabled={readOnly} />
          <span className="small">{b.enabled ? t("common.enabled") : t("common.disabled")}</span>
        </div>
      </header>

      {desc && (
        <div className="service-desc">
          {desc.includes("\n") ? (
            <>
              <button type="button" className="btn-link" aria-expanded={descOpen} onClick={() => setDescOpen((x) => !x)}>
                <Icon name="chevron" className={descOpen ? "rot90" : ""} />
                {descFirst}
              </button>
              {descOpen && <pre className="desc-text">{desc.split("\n").slice(1).join("\n").trim()}</pre>}
            </>
          ) : (
            <p>{desc}</p>
          )}
        </div>
      )}

      <div className="service-routes">
        <span className="label-like">{t("service.routes")}</span>
        {s.routes.length === 0 ? (
          <span className="muted small">{t("service.noRoutes")}</span>
        ) : (
          <ul className="route-chips">
            {s.routes.map((r, i) => (
              <li key={i} className={"route-chip" + (r.enabled ? "" : " off")}>
                <span className="route-fe">{r.frontend.name}</span>
                {r.isDefault ? (
                  <span>{t("service.defaultRoute")}</span>
                ) : (
                  <span className="mono">{routeText(r)}</span>
                )}
                {!r.enabled && <span className="small">({t("common.disabled")})</span>}
                {!r.isDefault && r.rule && !readOnly && (r.rule.kind === "host" || r.rule.kind === "path" || r.rule.kind === "host+path") && (
                  <button type="button" className="btn-icon btn-xs" aria-label={t("route.edit")} title={t("route.edit")} onClick={() => onDialog({ kind: "route", backend: b, route: r })}>
                    <Icon name="edit" size={13} />
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        {!readOnly && b.enabled && (
          <button type="button" className="btn btn-sm" onClick={() => onDialog({ kind: "route", backend: b })}>
            <Icon name="plus" size={14} /> {t("route.add")}
          </button>
        )}
      </div>

      <div className="table-wrap">
        <table className="servers">
          <caption className="sr-only">{t("service.serversOf", { name: b.name })}</caption>
          <thead>
            <tr>
              <th>{t("server.status")}</th>
              <th>{t("server.name")}</th>
              <th>{t("server.address")}</th>
              <th className="hide-sm">{t("server.check")}</th>
              <th className="hide-sm num">{t("server.sessions")}</th>
              <th>
                <span className="sr-only">{t("common.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {b.servers.length === 0 && (
              <tr>
                <td colSpan={6} className="muted">
                  {t("service.noServers")}
                </td>
              </tr>
            )}
            {b.servers.map((srv) => (
              <ServerRow key={srv.name + srv.line} b={b} srv={srv} stat={serverStatOf(stats, b.name, srv.name)} live={live} />
            ))}
          </tbody>
        </table>
      </div>

      {!readOnly && (
        <footer className="service-actions">
          <button type="button" className="btn btn-sm" onClick={() => onDialog({ kind: "server", backend: b })}>
            <Icon name="plus" size={14} /> {t("server.add")}
          </button>
          <button type="button" className="btn btn-sm" onClick={() => onDialog({ kind: "settings", backend: b })}>
            <Icon name="edit" size={14} /> {t("backend.settings")}
          </button>
          <button type="button" className="btn btn-sm" onClick={() => onDialog({ kind: "section", backend: b })}>
            <Icon name="code" size={14} /> {t("backend.raw")}
          </button>
          <span className="spacer" />
          <button
            type="button"
            className="btn btn-sm btn-danger-ghost"
            onClick={() => review({ title: t("backend.deleteTitle", { name: b.name }), ops: [{ op: "backend.delete", name: b.name }] })}
          >
            <Icon name="trash" size={14} /> {t("common.delete")}
          </button>
        </footer>
      )}
    </article>
  );
}

function routeText(r: RouteRef): string {
  const rule = r.rule!;
  if (rule.kind === "other" || rule.kind === "always") return rule.condTest ? `${rule.cond} ${rule.condTest}` : "*";
  const hosts = rule.hosts ?? [];
  const paths = rule.paths ?? [];
  if (hosts.length && paths.length) return hosts.flatMap((h) => paths.map((p) => h + p)).join(", ");
  if (hosts.length) return hosts.join(", ");
  return paths.map((p) => "*" + p).join(", ");
}

function ServerRow({ b, srv, stat, live }: { b: Backend; srv: Server; stat?: Stat; live: boolean }) {
  const { t, errorText } = useT();
  const { readOnly, refreshStatus } = useApp();
  const review = useReview();
  const toast = useToast();
  const inService = srv.enabled && !srv.maintenance;
  const liveState = stat?.status;
  const draining = liveState?.toUpperCase().startsWith("DRAIN");
  const canRuntime = live && b.enabled && srv.enabled && !!stat && !readOnly;

  const toggle = async (on: boolean) => {
    await review({
      title: on ? t("server.enableTitle", { name: srv.name }) : t("server.maintTitle", { name: srv.name }),
      ops: [{ op: "server.setEnabled", backend: b.name, name: srv.name, enabled: on }],
      compact: true,
    });
  };
  const runtime = async (state: "ready" | "drain") => {
    try {
      await api.runtimeServer(b.name, srv.name, state);
      toast({ tone: "ok", title: t(state === "drain" ? "server.drained" : "server.readied", { name: srv.name }), body: t("server.runtimeOnly") });
      await refreshStatus();
    } catch (e) {
      toast({ tone: "error", title: t("server.runtimeFailed"), body: errorText(e) });
    }
  };

  let statusCell;
  if (!srv.enabled) statusCell = <Badge tone="neutral">{t("server.commented")}</Badge>;
  else if (!b.enabled) statusCell = <Badge tone="neutral">{t("server.inactive")}</Badge>;
  else if (liveState) statusCell = <StatusBadge status={liveState} check={stat?.checkStatus} title={stat?.checkDesc || liveState} />;
  else if (srv.maintenance) statusCell = <StatusBadge status="MAINT" />;
  else statusCell = <Badge tone="neutral">{live ? t("server.notRunning") : t("status.unknown")}</Badge>;

  return (
    <tr className={inService ? "" : "row-off"}>
      <td>
        <div className="status-cell">
          {statusCell}
          {srv.maintenance && srv.enabled && <span className="tiny muted">{t("server.persisted")}</span>}
        </div>
      </td>
      <td className="mono">
        {srv.name}
        {srv.backup && (
          <Badge tone="neutral" title={t("server.backup")}>
            backup
          </Badge>
        )}
        {srv.ssl && <Badge tone="neutral">ssl</Badge>}
      </td>
      <td className="mono nowrap">
        {srv.address}
        {srv.port ? ":" + srv.port : ""}
      </td>
      <td className="hide-sm small">
        {srv.check ? (
          <span title={stat?.checkDesc}>
            {[srv.inter && `inter ${srv.inter}`, srv.fall && `fall ${srv.fall}`, srv.rise && `rise ${srv.rise}`].filter(Boolean).join(" · ") || t("server.check")}
            {stat?.checkStatus && <span className="muted"> · {stat.checkStatus}</span>}
          </span>
        ) : (
          <span className="muted">{t("server.noCheck")}</span>
        )}
      </td>
      <td className="hide-sm num">{stat ? stat.sessions : "–"}</td>
      <td>
        <div className="row-actions">
          {b.enabled && (
            <Switch
              checked={inService}
              onChange={toggle}
              disabled={readOnly}
              label={inService ? t("server.toMaint", { name: srv.name }) : t("server.toService", { name: srv.name })}
            />
          )}
          {canRuntime && (
            <button
              type="button"
              className="btn-icon"
              onClick={() => runtime(draining ? "ready" : "drain")}
              title={draining ? t("server.ready") : t("server.drain")}
              aria-label={(draining ? t("server.ready") : t("server.drain")) + " " + srv.name}
            >
              <Icon name={draining ? "play" : "drain"} />
            </button>
          )}
          {!readOnly && (
            <>
              <EditServerButton b={b} srv={srv} />
              <button
                type="button"
                className="btn-icon"
                title={t("server.delete")}
                aria-label={t("server.delete") + " " + srv.name}
                onClick={() => review({ title: t("server.deleteTitle", { name: srv.name, backend: b.name }), ops: [{ op: "server.delete", backend: b.name, name: srv.name }] })}
              >
                <Icon name="trash" />
              </button>
            </>
          )}
        </div>
      </td>
    </tr>
  );
}

function EditServerButton({ b, srv }: { b: Backend; srv: Server }) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" className="btn-icon" title={t("server.edit")} aria-label={t("server.edit") + " " + srv.name} onClick={() => setOpen(true)}>
        <Icon name="edit" />
      </button>
      {open && <ServerDialog backend={b} server={specOf(srv)} onClose={() => setOpen(false)} />}
    </>
  );
}
