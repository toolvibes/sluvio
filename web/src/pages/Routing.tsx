import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import type { Frontend, Rule } from "../api";
import { useT, type Key } from "../i18n";
import { useApp } from "../state";
import { Badge, Empty, Field, Icon, Spinner, Switch } from "../components/ui";
import { useReview } from "../components/review";
import { FrontendDialog, RouteDialog, SectionDialog } from "../components/dialogs";

type D = { kind: "route"; frontend?: Frontend; rule?: Rule } | { kind: "frontend" } | { kind: "section"; frontend: Frontend } | null;

export default function Routing() {
  const { t } = useT();
  const { config, readOnly } = useApp();
  const [dialog, setDialog] = useState<D>(null);
  if (!config) return <Spinner label={t("common.loading")} />;
  const fes = config.model.frontends;
  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>{t("routing.title")}</h1>
          <p className="muted">{t("routing.subtitle")}</p>
        </div>
        {!readOnly && (
          <div className="row-wrap">
            <button type="button" className="btn" onClick={() => setDialog({ kind: "frontend" })}>
              <Icon name="plus" /> {t("frontend.new")}
            </button>
            <button type="button" className="btn btn-primary" onClick={() => setDialog({ kind: "route" })}>
              <Icon name="plus" /> {t("route.add")}
            </button>
          </div>
        )}
      </div>
      <div className="notice notice-info">
        <Icon name="info" /> {t("routing.orderMatters")}
      </div>
      {fes.length === 0 && <Empty>{t("routing.none")}</Empty>}
      {fes.map((f) => (
        <FrontendCard key={f.name + f.line} f={f} onDialog={setDialog} />
      ))}
      {dialog?.kind === "route" && <RouteDialog frontend={dialog.frontend} rule={dialog.rule} onClose={() => setDialog(null)} />}
      {dialog?.kind === "frontend" && <FrontendDialog onClose={() => setDialog(null)} />}
      {dialog?.kind === "section" && <SectionDialog kind={dialog.frontend.kind} name={dialog.frontend.name} onClose={() => setDialog(null)} />}
    </div>
  );
}

function FrontendCard({ f, onDialog }: { f: Frontend; onDialog: (d: D) => void }) {
  const { t } = useT();
  const { config, readOnly } = useApp();
  const review = useReview();
  const backends = config?.model.backends ?? [];
  const known = new Set(backends.map((b) => b.name));
  const disabledBackends = new Set(backends.filter((b) => !b.enabled).map((b) => b.name));
  return (
    <section className={"card frontend" + (f.enabled ? "" : " is-disabled")} aria-labelledby={"fe-" + f.name}>
      <header className="frontend-head">
        <div>
          <h2 id={"fe-" + f.name} className="mono">
            {f.kind === "listen" ? "listen " : ""}
            {f.name}
          </h2>
          <div className="row-wrap">
            {!f.enabled && <Badge>{t("service.disabledCommented")}</Badge>}
            {f.mode && <Badge>{f.mode}</Badge>}
            {f.binds.map((b) => (
              <Badge key={b.line} tone={b.enabled ? "accent" : "neutral"} title={(b.params ?? []).join(" ")}>
                <span className="mono">
                  {b.address}
                  {b.ssl ? " · TLS" : ""}
                </span>
              </Badge>
            ))}
            <span className="muted small">{t("common.lineN", { n: f.line + 1 })}</span>
          </div>
        </div>
        {!readOnly && f.enabled && (
          <div className="row-wrap">
            <button type="button" className="btn btn-sm" onClick={() => onDialog({ kind: "route", frontend: f })}>
              <Icon name="plus" size={14} /> {t("route.add")}
            </button>
            <button type="button" className="btn btn-sm" onClick={() => onDialog({ kind: "section", frontend: f })}>
              <Icon name="code" size={14} /> {t("backend.raw")}
            </button>
          </div>
        )}
      </header>
      {f.description && <p className="muted small pre-line">{f.description}</p>}
      {f.rules.length === 0 ? (
        <p className="muted">{f.defaultBackend ? t("routing.noRules") : t("routing.noRulesNoDefault")}</p>
      ) : (
        <div className="table-wrap">
          <table className="rules">
            <thead>
              <tr>
                <th className="num">{t("routing.line")}</th>
                <th>{t("routing.match")}</th>
                <th>{t("route.backend")}</th>
                <th>{t("routing.state")}</th>
                <th>
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {f.rules.map((r, i) => {
                const simple = r.kind === "host" || r.kind === "path" || r.kind === "host+path";
                return (
                  <tr key={r.line} className={r.enabled ? "" : "row-off"}>
                    <td className="num muted mono">
                      <span className="order">{i + 1}.</span> {r.line + 1}
                    </td>
                    <td>
                      <div className="match">
                        <Badge tone="neutral">{t(("rule.kind." + r.kind) as Key)}</Badge>
                        {simple ? (
                          <span className="mono">
                            {(r.hosts ?? []).join(", ")}
                            {r.hosts?.length && r.paths?.length ? " + " : ""}
                            {(r.paths ?? []).join(", ")}
                          </span>
                        ) : (
                          <code className="mono small">{r.cond ? `${r.cond} ${r.condTest}` : t("rule.always")}</code>
                        )}
                      </div>
                    </td>
                    <td className="mono">
                      {known.has(r.backend) ? (
                        <Link to={`/services?q=${encodeURIComponent(r.backend)}`}>{r.backend}</Link>
                      ) : (
                        <span title={t("routing.unknownBackend")}>{r.backend}</span>
                      )}
                      {disabledBackends.has(r.backend) && <span className="muted small"> ({t("common.disabled")})</span>}
                    </td>
                    <td>
                      <Switch
                        checked={r.enabled}
                        disabled={readOnly || !f.enabled}
                        label={r.enabled ? t("route.disable", { line: r.line + 1 }) : t("route.enable", { line: r.line + 1 })}
                        onChange={(on) =>
                          review({
                            title: on ? t("route.enableTitle", { line: r.line + 1 }) : t("route.disableTitle", { line: r.line + 1 }),
                            ops: [{ op: "route.setEnabled", frontend: f.name, line: r.line, enabled: on }],
                            compact: true,
                          })
                        }
                      />
                    </td>
                    <td>
                      {!readOnly && f.enabled && (
                        <div className="row-actions">
                          {simple && (
                            <button type="button" className="btn-icon" title={t("route.edit")} aria-label={t("route.edit")} onClick={() => onDialog({ kind: "route", frontend: f, rule: r })}>
                              <Icon name="edit" />
                            </button>
                          )}
                          <button
                            type="button"
                            className="btn-icon"
                            title={t("route.delete")}
                            aria-label={t("route.delete")}
                            onClick={() => review({ title: t("route.deleteTitle", { line: r.line + 1 }), ops: [{ op: "route.delete", frontend: f.name, line: r.line }] })}
                          >
                            <Icon name="trash" />
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <DefaultBackend f={f} />
    </section>
  );
}

function DefaultBackend({ f }: { f: Frontend }) {
  const { t } = useT();
  const { config, readOnly } = useApp();
  const review = useReview();
  const [val, setVal] = useState(f.defaultBackend ?? "");
  useEffect(() => setVal(f.defaultBackend ?? ""), [f.defaultBackend]);
  const changed = val !== (f.defaultBackend ?? "");
  return (
    <div className="default-backend">
      <Field label={t("frontend.default")}>
        <div className="row-wrap">
          <select value={val} onChange={(e) => setVal(e.target.value)} disabled={readOnly || !f.enabled}>
            <option value="">{t("frontend.noDefault")}</option>
            {(config?.model.backends ?? [])
              .filter((b) => b.enabled || b.name === f.defaultBackend)
              .map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
          </select>
          {changed && (
            <button
              type="button"
              className="btn btn-sm btn-primary"
              onClick={async () => {
                const r = await review({
                  title: t("frontend.defaultTitle", { name: f.name }),
                  ops: [{ op: "frontend.setDefaultBackend", frontend: f.name, backend: val }],
                  compact: true,
                });
                if (!r) setVal(f.defaultBackend ?? "");
              }}
            >
              {t("common.review")}
            </button>
          )}
        </div>
      </Field>
    </div>
  );
}
