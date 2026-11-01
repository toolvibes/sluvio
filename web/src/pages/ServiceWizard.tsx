import { useMemo, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { NAME_RE, type Op, type ServerSpec } from "../api";
import { useT, type Key } from "../i18n";
import { useApp } from "../state";
import { backendPrefix, slug } from "../derive";
import { Field, Icon, ListInput, Spinner, TextField } from "../components/ui";
import { useReview } from "../components/review";
import { BALANCES, emptyServer, ServerFields, validateHost, validatePath, validateServer } from "../components/dialogs";

type Preset = "api" | "web" | "websocket" | "tcp";

const presets: Record<Preset, { icon: string; balance: string; hc?: { method: string; path: string; expect: string }; tunnel?: string; mode?: string }> = {
  api: { icon: "code", balance: "leastconn", hc: { method: "GET", path: "/healthz", expect: "status 200" } },
  web: { icon: "services", balance: "roundrobin", hc: { method: "GET", path: "/", expect: "status 200" } },
  websocket: { icon: "routing", balance: "leastconn", hc: { method: "GET", path: "/", expect: "status 200" }, tunnel: "3600s" },
  tcp: { icon: "setup", balance: "roundrobin", mode: "tcp" },
};

export default function ServiceWizard() {
  const { t } = useT();
  const { config, readOnly } = useApp();
  const review = useReview();
  const navigate = useNavigate();
  const model = config?.model;
  const prefix = useMemo(() => backendPrefix(model), [model]);
  const httpFrontends = (model?.frontends ?? []).filter((f) => f.enabled && f.mode !== "tcp");

  const [preset, setPreset] = useState<Preset>("api");
  const [title, setTitle] = useState("");
  const [name, setName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [description, setDescription] = useState("");
  const [frontend, setFrontend] = useState<string>("");
  const [hosts, setHosts] = useState<string[]>([]);
  const [paths, setPaths] = useState<string[]>([]);
  const [tcpBind, setTcpBind] = useState("");
  const [balance, setBalance] = useState(presets.api.balance);
  const [hcOn, setHcOn] = useState(true);
  const [hcPath, setHcPath] = useState(presets.api.hc!.path);
  const [hcMethod, setHcMethod] = useState("GET");
  const [hcExpect, setHcExpect] = useState("status 200");
  const [tunnel, setTunnel] = useState("");
  const [servers, setServers] = useState<ServerSpec[]>([emptyServer("")]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [serverErrors, setServerErrors] = useState<Record<string, string>[]>([]);

  if (!config || !model) return <Spinner label={t("common.loading")} />;
  const fe = frontend || httpFrontends.find((f) => f.rules.length > 0)?.name || httpFrontends[0]?.name || "";
  const effectiveName = nameTouched ? name : title ? prefix + slug(title) : "";
  const serverBase = slug(title) || "srv";

  const choose = (p: Preset) => {
    setPreset(p);
    const pr = presets[p];
    setBalance(pr.balance);
    setHcOn(!!pr.hc);
    if (pr.hc) {
      setHcMethod(pr.hc.method);
      setHcPath(pr.hc.path);
      setHcExpect(pr.hc.expect);
    }
    setTunnel(pr.tunnel ?? "");
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (!NAME_RE.test(effectiveName)) errs.name = t("val.name");
    else if (model.backends.some((b) => b.name === effectiveName)) errs.name = t("val.duplicate");
    if (preset === "tcp") {
      if (!/^[^\s#]+$/.test(tcpBind)) errs.bind = t("val.required");
    } else {
      if (!fe) errs.frontend = t("val.required");
      if (hosts.length === 0 && paths.length === 0) errs.route = t("err.route_empty");
    }
    if (hcOn && !hcPath.startsWith("/")) errs.hc = t("val.path");
    const named = servers.map((s, i) => ({ ...s, name: s.name || `${serverBase}-${i + 1}` }));
    const sErrs = named.map((s, i) => validateServer(s, t, named.slice(0, i).map((x) => x.name)));
    if (named.length === 0) errs.servers = t("val.oneServer");
    setErrors(errs);
    setServerErrors(sErrs);
    if (Object.keys(errs).length || sErrs.some((x) => Object.keys(x).length)) {
      document.querySelector(".has-error, .notice-error")?.scrollIntoView({ block: "center", behavior: "smooth" });
      return;
    }
    const isTcp = preset === "tcp";
    const ops: Op[] = [
      {
        op: "backend.create",
        backendSpec: {
          name: effectiveName,
          description: description.trim() || title.trim() || undefined,
          mode: isTcp ? "tcp" : undefined,
          balance,
          healthCheck: !isTcp && hcOn ? { method: hcMethod, path: hcPath, expect: hcExpect || undefined } : undefined,
          timeouts: tunnel ? { tunnel } : undefined,
          servers: named,
        },
      },
    ];
    if (isTcp) {
      ops.push({ op: "frontend.create", frontendSpec: { name: "fe-" + slug(title || effectiveName), mode: "tcp", binds: [tcpBind], defaultBackend: effectiveName } });
    } else {
      ops.push({ op: "route.create", route: { frontend: fe, backend: effectiveName, hosts, paths } });
    }
    const r = await review({ title: t("wizard.reviewTitle", { name: effectiveName }), ops, defaultMessage: t("wizard.message", { name: effectiveName }) });
    if (r) navigate(`/services?q=${encodeURIComponent(effectiveName)}`);
  };

  return (
    <div className="page page-narrow">
      <div className="page-head">
        <div>
          <Link to="/services" className="btn-link back">
            ← {t("nav.services")}
          </Link>
          <h1>{t("wizard.title")}</h1>
          <p className="muted">{t("wizard.subtitle")}</p>
        </div>
      </div>
      <form onSubmit={submit} noValidate className="wizard">
        <section className="card">
          <h2 className="card-title">
            <span className="step">1</span> {t("wizard.type")}
          </h2>
          <div className="preset-grid" role="radiogroup" aria-label={t("wizard.type")}>
            {(Object.keys(presets) as Preset[]).map((p) => (
              <button key={p} type="button" role="radio" aria-checked={preset === p} className={"preset" + (preset === p ? " active" : "")} onClick={() => choose(p)}>
                <Icon name={presets[p].icon} size={20} />
                <strong>{t(("wizard.preset." + p) as Key)}</strong>
                <span className="muted small">{t(("wizard.preset." + p + "Hint") as Key)}</span>
              </button>
            ))}
          </div>
        </section>

        <section className="card">
          <h2 className="card-title">
            <span className="step">2</span> {t("wizard.naming")}
          </h2>
          <div className="form-grid">
            <TextField label={t("wizard.displayName")} value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t("wizard.displayNamePh")} />
            <TextField
              label={t("wizard.backendName")}
              value={effectiveName}
              onChange={(e) => {
                setNameTouched(true);
                setName(e.target.value);
              }}
              error={errors.name}
              hint={t("wizard.backendNameHint", { prefix: prefix || "–" })}
              className="mono"
              spellCheck={false}
            />
            <Field label={t("wizard.description")} className="span-2" hint={t("wizard.descriptionHint")}>
              <textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} placeholder={t("wizard.descriptionPh")} />
            </Field>
          </div>
        </section>

        <section className="card">
          <h2 className="card-title">
            <span className="step">3</span> {t("wizard.reach")}
          </h2>
          {preset === "tcp" ? (
            <div className="form-grid">
              <TextField label={t("wizard.tcpBind")} value={tcpBind} onChange={(e) => setTcpBind(e.target.value.trim())} placeholder="*:5432" error={errors.bind} hint={t("wizard.tcpBindHint")} className="mono" />
            </div>
          ) : (
            <div className="form-grid">
              {errors.route && <div className="notice notice-error span-2">{errors.route}</div>}
              <Field label={t("route.frontend")} error={errors.frontend} hint={t("wizard.frontendHint")}>
                <select value={fe} onChange={(e) => setFrontend(e.target.value)}>
                  {httpFrontends.map((f) => (
                    <option key={f.name} value={f.name}>
                      {f.name} ({f.binds.map((b) => b.address).join(", ")})
                    </option>
                  ))}
                </select>
              </Field>
              <div />
              <div className="span-2">
                <ListInput label={t("route.hosts")} values={hosts} onChange={setHosts} placeholder="shop.example.io" validate={validateHost(t)} hint={t("route.hostsHint")} mono />
              </div>
              <div className="span-2">
                <ListInput label={t("route.paths")} values={paths} onChange={setPaths} placeholder="/api/" validate={validatePath(t)} hint={t("route.pathsHint")} mono />
              </div>
            </div>
          )}
        </section>

        <section className="card">
          <h2 className="card-title">
            <span className="step">4</span> {t("wizard.servers")}
          </h2>
          {errors.servers && <div className="notice notice-error">{errors.servers}</div>}
          <div className="server-rows">
            {servers.map((s, i) => (
              <div className="server-row" key={i}>
                <ServerFields
                  value={{ ...s, name: s.name }}
                  onChange={(v) => setServers(servers.map((x, k) => (k === i ? v : x)))}
                  errors={serverErrors[i] ?? {}}
                  compact
                />
                <div className="server-row-meta">
                  <span className="muted small">{s.name ? "" : t("wizard.autoName", { name: `${serverBase}-${i + 1}` })}</span>
                  {servers.length > 1 && (
                    <button type="button" className="btn btn-sm btn-danger-ghost" onClick={() => setServers(servers.filter((_, k) => k !== i))}>
                      <Icon name="trash" size={14} /> {t("common.remove")}
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
          <button
            type="button"
            className="btn btn-sm"
            onClick={() => {
              const last = servers[servers.length - 1];
              setServers([...servers, { ...emptyServer(""), port: last?.port ?? "", inter: last?.inter, fall: last?.fall, rise: last?.rise }]);
            }}
          >
            <Icon name="plus" size={14} /> {t("server.add")}
          </button>
        </section>

        <section className="card">
          <h2 className="card-title">
            <span className="step">5</span> {t("wizard.behaviour")}
          </h2>
          <div className="form-grid">
            <Field label={t("backend.balance")} hint={t("wizard.balanceHint")}>
              <select value={balance} onChange={(e) => setBalance(e.target.value)}>
                {BALANCES.map((b) => (
                  <option key={b}>{b}</option>
                ))}
              </select>
            </Field>
            {preset !== "tcp" ? (
              <TextField label="timeout tunnel" value={tunnel} onChange={(e) => setTunnel(e.target.value.trim())} placeholder="3600s" hint={t("wizard.tunnelHint")} className="mono" />
            ) : (
              <div />
            )}
            {preset !== "tcp" && (
              <fieldset className="span-2 fieldset">
                <legend>
                  <label className="checkbox">
                    <input type="checkbox" checked={hcOn} onChange={(e) => setHcOn(e.target.checked)} />
                    {t("backend.httpCheck")}
                  </label>
                </legend>
                {hcOn && (
                  <div className="row-fields">
                    <Field label={t("backend.method")} className="f-small">
                      <select value={hcMethod} onChange={(e) => setHcMethod(e.target.value)}>
                        {["GET", "HEAD", "OPTIONS"].map((m) => (
                          <option key={m}>{m}</option>
                        ))}
                      </select>
                    </Field>
                    <TextField label={t("backend.path")} value={hcPath} onChange={(e) => setHcPath(e.target.value)} className="mono f-grow" error={errors.hc} />
                    <TextField label={t("backend.expect")} value={hcExpect} onChange={(e) => setHcExpect(e.target.value)} className="mono" />
                  </div>
                )}
              </fieldset>
            )}
          </div>
        </section>

        <div className="wizard-foot">
          <Link to="/services" className="btn">
            {t("common.cancel")}
          </Link>
          <button type="submit" className="btn btn-primary" disabled={readOnly}>
            <Icon name="check" /> {t("wizard.submit")}
          </button>
        </div>
      </form>
    </div>
  );
}
