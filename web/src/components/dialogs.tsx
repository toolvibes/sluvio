import { useEffect, useId, useState, type FormEvent } from "react";
import { api, HOST_RE, NAME_RE, PORT_RE, type Backend, type Frontend, type Rule, type ServerSpec } from "../api";
import { useT, type Key } from "../i18n";

type T = (k: Key, p?: Record<string, string | number | undefined>) => string;
import { useApp } from "../state";
import { Dialog, Field, Icon, ListInput, Spinner, TextField } from "./ui";
import { useReview } from "./review";

// ---------------------------------------------------------------------------
// server add / edit

export function emptyServer(name = ""): ServerSpec {
  return { name, address: "", port: "", check: true, inter: "5s", fall: "3", rise: "2", weight: "", backup: false, ssl: false, verify: "none", extra: [] };
}

export function validateServer(s: ServerSpec, tt: T, names: string[] = []): Record<string, string> {
  const e: Record<string, string> = {};
  if (!NAME_RE.test(s.name)) e.name = tt("val.name");
  else if (names.includes(s.name)) e.name = tt("val.duplicate");
  if (!s.address.trim() || /[\s#]/.test(s.address)) e.address = tt("val.address");
  if (s.port && !PORT_RE.test(s.port)) e.port = tt("val.port");
  for (const k of ["inter", "fall", "rise", "weight"] as const) {
    const v = s[k];
    if (v && /[\s#]/.test(v)) e[k] = tt("val.value");
  }
  return e;
}

export function ServerFields({
  value,
  onChange,
  errors,
  compact,
}: {
  value: ServerSpec;
  onChange: (s: ServerSpec) => void;
  errors: Record<string, string>;
  compact?: boolean;
}) {
  const { t } = useT();
  const set = (patch: Partial<ServerSpec>) => onChange({ ...value, ...patch });
  const sslId = useId();
  const checkId = useId();
  const backupId = useId();
  return (
    <div className={"server-fields" + (compact ? " compact" : "")}>
      <TextField label={t("server.name")} value={value.name} onChange={(e) => set({ name: e.target.value })} error={errors.name} className="f-name" spellCheck={false} />
      <TextField
        label={t("server.address")}
        value={value.address}
        onChange={(e) => set({ address: e.target.value.trim() })}
        error={errors.address}
        className="f-addr mono"
        placeholder="10.0.0.5"
        spellCheck={false}
      />
      <TextField label={t("server.port")} value={value.port} onChange={(e) => set({ port: e.target.value.trim() })} error={errors.port} className="f-port mono" placeholder="8080" inputMode="numeric" />
      <TextField label={t("server.weight")} value={value.weight ?? ""} onChange={(e) => set({ weight: e.target.value.trim() })} error={errors.weight} className="f-small" placeholder="100" inputMode="numeric" />
      <div className="field f-check">
        <span className="label-like">{t("server.healthCheck")}</span>
        <label className="checkbox" htmlFor={checkId}>
          <input id={checkId} type="checkbox" checked={value.check} onChange={(e) => set({ check: e.target.checked })} />
          {t("server.check")}
        </label>
      </div>
      {value.check && (
        <>
          <TextField label={t("server.inter")} value={value.inter ?? ""} onChange={(e) => set({ inter: e.target.value.trim() })} error={errors.inter} className="f-small mono" placeholder="2s" />
          <TextField label={t("server.fall")} value={value.fall ?? ""} onChange={(e) => set({ fall: e.target.value.trim() })} error={errors.fall} className="f-small mono" placeholder="3" />
          <TextField label={t("server.rise")} value={value.rise ?? ""} onChange={(e) => set({ rise: e.target.value.trim() })} error={errors.rise} className="f-small mono" placeholder="2" />
        </>
      )}
      <div className="field f-flags">
        <span className="label-like">{t("server.options")}</span>
        <div className="row-wrap">
          <label className="checkbox" htmlFor={backupId}>
            <input id={backupId} type="checkbox" checked={!!value.backup} onChange={(e) => set({ backup: e.target.checked })} />
            {t("server.backup")}
          </label>
          <label className="checkbox" htmlFor={sslId}>
            <input id={sslId} type="checkbox" checked={!!value.ssl} onChange={(e) => set({ ssl: e.target.checked })} />
            {t("server.ssl")}
          </label>
          {value.ssl && (
            <select aria-label={t("server.verify")} value={value.verify || "none"} onChange={(e) => set({ verify: e.target.value })}>
              <option value="none">{t("server.verifyNone")}</option>
              <option value="required">{t("server.verifyRequired")}</option>
            </select>
          )}
        </div>
      </div>
      {!compact && (value.extra?.length ?? 0) > 0 && (
        <TextField
          label={t("server.extra")}
          value={(value.extra ?? []).join(" ")}
          onChange={(e) => set({ extra: e.target.value.split(/\s+/).filter(Boolean) })}
          className="f-wide mono"
          hint={t("server.extraHint")}
        />
      )}
    </div>
  );
}

export function ServerDialog({ backend, server, onClose }: { backend: Backend; server?: ServerSpec; onClose: () => void }) {
  const { t } = useT();
  const review = useReview();
  const [v, setV] = useState<ServerSpec>(server ?? emptyServer(suggestServerName(backend)));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const others = backend.servers.map((s) => s.name).filter((n) => n !== server?.name);
    const errs = validateServer(v, t, others);
    setErrors(errs);
    if (Object.keys(errs).length) return;
    const op = server
      ? { op: "server.update", backend: backend.name, name: server.name, server: v }
      : { op: "server.add", backend: backend.name, server: v };
    const r = await review({ title: server ? t("server.editTitle", { name: server.name, backend: backend.name }) : t("server.addTitle", { backend: backend.name }), ops: [op] });
    if (r) onClose();
  };
  return (
    <Dialog
      title={server ? t("server.editTitle", { name: server.name, backend: backend.name }) : t("server.addTitle", { backend: backend.name })}
      onClose={onClose}
      wide
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" form="server-form" className="btn btn-primary">
            {t("common.review")}
          </button>
        </>
      }
    >
      <form id="server-form" onSubmit={submit} noValidate>
        <ServerFields value={v} onChange={setV} errors={errors} />
      </form>
    </Dialog>
  );
}

function suggestServerName(b: Backend): string {
  const base = b.servers[0]?.name.replace(/-?\d+$/, "") || b.name.replace(/^(backend|be|bk)[-_]/, "");
  for (let i = b.servers.length + 1; i < 100; i++) {
    const n = `${base}-${i}`;
    if (!b.servers.some((s) => s.name === n)) return n;
  }
  return base;
}

// ---------------------------------------------------------------------------
// backend settings

export const BALANCES = ["roundrobin", "leastconn", "source", "first", "static-rr", "uri", "random"];

export function BackendDialog({ backend, onClose }: { backend: Backend; onClose: () => void }) {
  const { t } = useT();
  const review = useReview();
  const [mode, setMode] = useState(backend.mode ?? "");
  const [balance, setBalance] = useState(backend.balance ?? "");
  const [hcOn, setHcOn] = useState(!!backend.healthCheck);
  const [method, setMethod] = useState(backend.healthCheck?.method ?? "GET");
  const [path, setPath] = useState(backend.healthCheck?.path ?? "/");
  const [expect, setExpect] = useState(backend.healthCheck?.expect ?? "status 200");
  const [timeouts, setTimeouts] = useState<Record<string, string>>({ ...(backend.timeouts ?? {}) });
  const [err, setErr] = useState<string | null>(null);
  const hcId = useId();
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (hcOn && path && !path.startsWith("/")) {
      setErr(t("val.path"));
      return;
    }
    const cleaned: Record<string, string> = {};
    for (const [k, v] of Object.entries(timeouts)) if (v.trim()) cleaned[k] = v.trim();
    const r = await review({
      title: t("backend.settingsTitle", { name: backend.name }),
      ops: [
        {
          op: "backend.update",
          name: backend.name,
          backendSpec: {
            name: backend.name,
            mode: mode || undefined,
            balance: balance || undefined,
            healthCheck: hcOn ? { method, path, expect: expect.trim() || undefined } : undefined,
            timeouts: cleaned,
          },
        },
      ],
    });
    if (r) onClose();
  };
  const known = ["connect", "server", "tunnel"];
  const otherTimeouts = Object.keys(timeouts).filter((k) => !known.includes(k));
  return (
    <Dialog
      title={t("backend.settingsTitle", { name: backend.name })}
      onClose={onClose}
      wide
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" form="backend-form" className="btn btn-primary">
            {t("common.review")}
          </button>
        </>
      }
    >
      <form id="backend-form" onSubmit={submit} className="form-grid" noValidate>
        {err && <div className="notice notice-error span-2">{err}</div>}
        <Field label={t("backend.mode")} hint={t("backend.modeHint")}>
          <select value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="">{t("backend.modeDefault")}</option>
            <option value="http">http</option>
            <option value="tcp">tcp</option>
          </select>
        </Field>
        <Field label={t("backend.balance")}>
          <select value={balance} onChange={(e) => setBalance(e.target.value)}>
            <option value="">{t("backend.balanceDefault")}</option>
            {BALANCES.concat(balance && !BALANCES.includes(balance) ? [balance] : []).map((b) => (
              <option key={b} value={b}>
                {b}
              </option>
            ))}
          </select>
        </Field>
        <fieldset className="span-2 fieldset">
          <legend>
            <label className="checkbox" htmlFor={hcId}>
              <input id={hcId} type="checkbox" checked={hcOn} onChange={(e) => setHcOn(e.target.checked)} />
              {t("backend.httpCheck")}
            </label>
          </legend>
          {hcOn && (
            <div className="row-fields">
              <Field label={t("backend.method")} className="f-small">
                <select value={method} onChange={(e) => setMethod(e.target.value)}>
                  {["GET", "HEAD", "OPTIONS", "POST"].map((m) => (
                    <option key={m}>{m}</option>
                  ))}
                </select>
              </Field>
              <TextField label={t("backend.path")} value={path} onChange={(e) => setPath(e.target.value)} className="mono f-grow" placeholder="/healthz" />
              <TextField label={t("backend.expect")} value={expect} onChange={(e) => setExpect(e.target.value)} className="mono" placeholder="status 200" />
            </div>
          )}
          <p className="field-hint">{t("backend.httpCheckHint")}</p>
        </fieldset>
        <fieldset className="span-2 fieldset">
          <legend>{t("backend.timeouts")}</legend>
          <div className="row-fields">
            {known.concat(otherTimeouts).map((k) => (
              <TextField
                key={k}
                label={`timeout ${k}`}
                value={timeouts[k] ?? ""}
                onChange={(e) => setTimeouts({ ...timeouts, [k]: e.target.value })}
                className="mono f-small"
                placeholder={k === "tunnel" ? "3600s" : "–"}
              />
            ))}
          </div>
          <p className="field-hint">{t("backend.timeoutsHint")}</p>
        </fieldset>
      </form>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// raw section editor

export function SectionDialog({ kind, name, onClose }: { kind: string; name: string; onClose: () => void }) {
  const { t, errorText } = useT();
  const review = useReview();
  const [content, setContent] = useState<string | null>(null);
  const [orig, setOrig] = useState("");
  const [version, setVersion] = useState("");
  const [err, setErr] = useState<unknown>(null);
  useEffect(() => {
    api
      .section(kind, name)
      .then((r) => {
        setContent(r.content);
        setOrig(r.content);
        setVersion(r.version);
      })
      .catch(setErr);
  }, [kind, name]);
  const submit = async () => {
    if (content === null) return;
    const r = await review({ title: t("section.title", { kind, name }), ops: [{ op: "section.replace", kind, name, content }], version });
    if (r) onClose();
  };
  return (
    <Dialog
      title={t("section.title", { kind, name })}
      onClose={onClose}
      wide
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="button" className="btn btn-primary" disabled={content === null || content === orig} onClick={submit}>
            {t("common.review")}
          </button>
        </>
      }
    >
      {err != null && <div className="notice notice-error">{errorText(err)}</div>}
      {content === null && err == null && <Spinner />}
      {content !== null && (
        <>
          <p className="muted small">{t("section.hint")}</p>
          <textarea
            className="code-input"
            value={content}
            spellCheck={false}
            rows={Math.min(30, Math.max(8, content.split("\n").length + 2))}
            onChange={(e) => setContent(e.target.value)}
            aria-label={t("section.title", { kind, name })}
          />
        </>
      )}
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// enable / disable a service

export function DisableServiceDialog({ backend, defaultOf, onClose }: { backend: Backend; defaultOf: string[]; onClose: () => void }) {
  const { t } = useT();
  const review = useReview();
  const [keep, setKeep] = useState(defaultOf.length > 0);
  const go = async () => {
    const r = await review({
      title: t("service.disableTitle", { name: backend.name }),
      ops: [{ op: "service.setEnabled", backend: backend.name, enabled: false, keepBackend: keep }],
      compact: true,
    });
    if (r) onClose();
  };
  return (
    <Dialog
      title={t("service.disableTitle", { name: backend.name })}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="button" className="btn btn-danger" onClick={go} data-autofocus>
            {t("common.review")}
          </button>
        </>
      }
    >
      <div className="radio-list">
        <label className={"radio-card" + (defaultOf.length ? " disabled" : "")}>
          <input type="radio" name="keep" checked={!keep} disabled={defaultOf.length > 0} onChange={() => setKeep(false)} />
          <div>
            <strong>{t("service.disableFull")}</strong>
            <div className="muted small">{t("service.disableFullHint")}</div>
          </div>
        </label>
        <label className="radio-card">
          <input type="radio" name="keep" checked={keep} onChange={() => setKeep(true)} />
          <div>
            <strong>{t("service.disableRoutes")}</strong>
            <div className="muted small">{t("service.disableRoutesHint")}</div>
          </div>
        </label>
      </div>
      {defaultOf.length > 0 && (
        <div className="notice notice-warn">
          <Icon name="warn" /> {t("service.isDefault", { frontends: defaultOf.join(", ") })}
        </div>
      )}
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// routes

export function validateHost(tt: T) {
  return (v: string) => (HOST_RE.test(v.toLowerCase()) ? null : tt("val.host"));
}

export function validatePath(tt: T) {
  return (v: string) => (v.startsWith("/") && !/[\s#]/.test(v) ? null : tt("val.path"));
}

export function RouteDialog({
  frontend,
  rule,
  defaultBackend,
  onClose,
}: {
  frontend?: Frontend;
  rule?: Rule;
  defaultBackend?: string;
  onClose: () => void;
}) {
  const { t } = useT();
  const { config } = useApp();
  const review = useReview();
  const frontends = (config?.model.frontends ?? []).filter((f) => f.enabled && f.mode !== "tcp");
  const [fe, setFe] = useState(frontend?.name ?? frontends.find((f) => f.rules.length > 0)?.name ?? frontends[0]?.name ?? "");
  const [backend, setBackend] = useState(rule?.backend ?? defaultBackend ?? "");
  const [hosts, setHosts] = useState<string[]>(rule?.hosts ?? []);
  const [paths, setPaths] = useState<string[]>(rule?.paths ?? []);
  const [err, setErr] = useState<string | null>(null);
  const backends = config?.model.backends ?? [];
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!fe || !backend) {
      setErr(t("val.required"));
      return;
    }
    if (hosts.length === 0 && paths.length === 0) {
      setErr(t("err.route_empty"));
      return;
    }
    const route = { frontend: fe, backend, hosts, paths };
    const op = rule ? { op: "route.update", frontend: fe, line: rule.line, route } : { op: "route.create", route };
    const r = await review({ title: rule ? t("route.editTitle", { line: rule.line + 1, frontend: fe }) : t("route.addTitle", { frontend: fe }), ops: [op] });
    if (r) onClose();
  };
  return (
    <Dialog
      title={rule ? t("route.editTitle", { line: rule.line + 1, frontend: fe }) : t("route.addTitle", { frontend: fe || "…" })}
      onClose={onClose}
      wide
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" form="route-form" className="btn btn-primary">
            {t("common.review")}
          </button>
        </>
      }
    >
      <form id="route-form" onSubmit={submit} className="form-grid" noValidate>
        {err && <div className="notice notice-error span-2">{err}</div>}
        <Field label={t("route.frontend")}>
          <select value={fe} onChange={(e) => setFe(e.target.value)} disabled={!!rule}>
            {frontends.map((f) => (
              <option key={f.name} value={f.name}>
                {f.name} ({f.binds.map((b) => b.address).join(", ")})
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("route.backend")}>
          <select value={backend} onChange={(e) => setBackend(e.target.value)}>
            <option value="">–</option>
            {backends.map((b) => (
              <option key={b.name} value={b.name}>
                {b.name}
                {b.enabled ? "" : ` (${t("common.disabled")})`}
              </option>
            ))}
          </select>
        </Field>
        <div className="span-2">
          <ListInput label={t("route.hosts")} values={hosts} onChange={setHosts} placeholder="app.example.com" validate={validateHost(t)} hint={t("route.hostsHint")} mono />
        </div>
        <div className="span-2">
          <ListInput label={t("route.paths")} values={paths} onChange={setPaths} placeholder="/api/" validate={validatePath(t)} hint={t("route.pathsHint")} mono />
        </div>
        <p className="muted small span-2">{t("route.orderHint")}</p>
      </form>
    </Dialog>
  );
}

export function FrontendDialog({ onClose }: { onClose: () => void }) {
  const { t } = useT();
  const { config } = useApp();
  const review = useReview();
  const [name, setName] = useState("");
  const [binds, setBinds] = useState<string[]>([]);
  const [mode, setMode] = useState("");
  const [def, setDef] = useState("");
  const [err, setErr] = useState<Record<string, string>>({});
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (!NAME_RE.test(name)) errs.name = t("val.name");
    if (binds.length === 0) errs.binds = t("val.required");
    setErr(errs);
    if (Object.keys(errs).length) return;
    const r = await review({
      title: t("frontend.newTitle"),
      ops: [{ op: "frontend.create", frontendSpec: { name, binds, mode: mode || undefined, defaultBackend: def || undefined } }],
    });
    if (r) onClose();
  };
  return (
    <Dialog
      title={t("frontend.newTitle")}
      onClose={onClose}
      wide
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" form="fe-form" className="btn btn-primary">
            {t("common.review")}
          </button>
        </>
      }
    >
      <form id="fe-form" onSubmit={submit} className="form-grid" noValidate>
        <TextField label={t("frontend.name")} value={name} onChange={(e) => setName(e.target.value)} error={err.name} className="mono" />
        <Field label={t("backend.mode")}>
          <select value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="">{t("backend.modeDefault")}</option>
            <option value="http">http</option>
            <option value="tcp">tcp</option>
          </select>
        </Field>
        <div className="span-2">
          <ListInput
            label={t("frontend.binds")}
            values={binds}
            onChange={setBinds}
            placeholder="*:8080"
            validate={(v) => (/^[^\s#]+$/.test(v) ? null : t("val.value"))}
            hint={err.binds ?? t("frontend.bindsHint")}
            mono
          />
        </div>
        <Field label={t("frontend.default")} className="span-2">
          <select value={def} onChange={(e) => setDef(e.target.value)}>
            <option value="">–</option>
            {(config?.model.backends ?? [])
              .filter((b) => b.enabled)
              .map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
          </select>
        </Field>
      </form>
    </Dialog>
  );
}
