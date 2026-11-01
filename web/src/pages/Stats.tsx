import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import type { Stat } from "../api";
import { useT, type Key } from "../i18n";
import { useApp, type Sample } from "../state";
import { Badge, Empty, Icon, StatusBadge } from "../components/ui";
import { CodeBar, LineChart, Sparkline, type CodeCounts } from "../components/charts";
import {
  currentRate,
  errorCount,
  fmtBytes,
  fmtCompact,
  fmtDuration,
  fmtInt,
  fmtMs,
  fmtRate,
  isProblem,
  rateSeries,
  responses,
  rowKey,
  sumRateSeries,
  trafficCounter,
} from "../stats";

const INTERVALS = [2000, 5000, 10000, 0];
const SPARK_POINTS = 30;

type Filter = "all" | "frontends" | "backends";

export default function Stats() {
  const { t, lang } = useT();
  const { status, samples, pollMs, setPollMs, refreshStatus } = useApp();
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState(params.get("q") ?? "");
  const [filter, setFilter] = useState<Filter>("all");
  const [problemsOnly, setProblemsOnly] = useState(false);
  const [allCols, setAllCols] = useState(false);
  const [chartFe, setChartFe] = useState("");

  // Faster updates while this page is open; restore the default when leaving.
  useEffect(() => {
    setPollMs(2000);
    void refreshStatus();
    return () => setPollMs(5000);
  }, [setPollMs, refreshStatus]);

  useEffect(() => {
    const next = new URLSearchParams(params);
    if (q) next.set("q", q);
    else next.delete("q");
    if (next.toString() !== params.toString()) setParams(next, { replace: true });
  }, [q, params, setParams]);

  const stats = useMemo(() => (status?.available ? status.stats : []), [status]);
  const frontends = stats.filter((s) => s.type === "frontend");
  const frontendKeys = frontends.map(rowKey);
  const info = status?.info;

  // KPIs over the collected samples.
  const reqNow = useMemo(() => sumRateSeries(samples.slice(-2), frontendKeys, trafficCounter)[0]?.v ?? null, [samples, frontendKeys]);
  const win = samples.length >= 2 ? [samples[0], samples[samples.length - 1]] : null;
  const delta = (pick: (s: Stat) => number) => {
    if (!win) return null;
    let d = 0;
    for (const k of frontendKeys) {
      const a = win[0].status.stats.find((x) => rowKey(x) === k);
      const b = win[1].status.stats.find((x) => rowKey(x) === k);
      if (a && b && pick(b) >= pick(a)) d += pick(b) - pick(a);
    }
    return d;
  };
  const d5xx = delta((s) => s.http5xx);
  const dResp = delta(responses);
  const errShare = dResp && dResp > 0 && d5xx !== null ? (d5xx / dResp) * 100 : null;
  const total5xx = frontends.reduce((n, s) => n + s.http5xx, 0);
  const totalResp = frontends.reduce((n, s) => n + responses(s), 0);
  const bytesNow = useMemo(
    () => sumRateSeries(samples.slice(-2), frontendKeys, (s) => s.bytesIn + s.bytesOut)[0]?.v ?? null,
    [samples, frontendKeys],
  );
  const bytesTotal = frontends.reduce((n, s) => n + s.bytesIn + s.bytesOut, 0);

  // Chart series.
  const chartKeys = chartFe ? [`${chartFe}/FRONTEND`] : frontendKeys;
  const reqSeries = useMemo(() => sumRateSeries(samples, chartKeys, trafficCounter), [samples, chartKeys.join()]);
  const errSeries = useMemo(() => sumRateSeries(samples, chartKeys, (s) => s.http5xx), [samples, chartKeys.join()]);
  const timeFmt = useMemo(() => {
    const f = new Intl.DateTimeFormat(lang, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    return (ts: number) => f.format(new Date(ts));
  }, [lang]);

  // Filtering of tables.
  const needle = q.trim().toLowerCase();
  const matches = (s: Stat) =>
    !needle || s.proxy.toLowerCase().includes(needle) || s.name.toLowerCase().includes(needle) || (s.addr ?? "").includes(needle);
  const backendNames = Array.from(new Set(stats.filter((s) => s.type === "backend").map((s) => s.proxy)));
  const backendGroups = backendNames
    .map((name) => {
      const rows = stats.filter((s) => s.proxy === name && (s.type === "server" || s.type === "backend"));
      const servers = rows.filter((s) => s.type === "server");
      const total = rows.find((s) => s.type === "backend")!;
      return { name, servers, total };
    })
    .filter((g) => {
      const rows = [g.total, ...g.servers];
      if (problemsOnly && !rows.some(isProblem)) return false;
      return rows.some(matches);
    });
  const feRows = frontends.filter((s) => (!problemsOnly || isProblem(s)) && matches(s));

  if (!status?.available) {
    return (
      <div className="page">
        <div className="page-head">
          <div>
            <h1>{t("stats.title")}</h1>
          </div>
        </div>
        <Empty>
          {t("stats.noLive")} <Link to="/setup">{t("nav.setup")}</Link>
        </Empty>
      </div>
    );
  }

  const counters = info ? t("stats.subtitle", { v: fmtDuration(info.uptimeSec) }) : "";
  const showFe = filter !== "backends";
  const showBe = filter !== "frontends";

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>{t("stats.title")}</h1>
          <p>{counters}</p>
        </div>
        <div className="seg" role="group" aria-label={t("stats.interval")}>
          {INTERVALS.map((ms) => (
            <button key={ms} type="button" className={pollMs === ms ? "active" : ""} aria-pressed={pollMs === ms} onClick={() => setPollMs(ms)}>
              {ms === 0 ? (
                <>
                  <Icon name="pause" /> {t("stats.pause")}
                </>
              ) : (
                `${ms / 1000} s`
              )}
            </button>
          ))}
        </div>
      </div>

      <div className="tiles">
        <div className="tile">
          <div className="tile-label">{t("stats.kpi.reqRate")}</div>
          <div className="tile-value">{fmtRate(reqNow, lang)}</div>
          <div className="tile-sub muted">{t("stats.kpi.reqRateSub", { n: fmtCompact(info?.cumReq, lang) })}</div>
        </div>
        <div className="tile">
          <div className="tile-label">{t("stats.kpi.conns")}</div>
          <div className="tile-value">{info?.currConns ?? "–"}</div>
          <div className="tile-sub muted">{t("stats.kpi.connsSub", { n: fmtInt(info?.connRate, lang), max: info?.maxConn ?? "–" })}</div>
        </div>
        <div className="tile">
          <div className="tile-label">{t("stats.kpi.errRate")}</div>
          <div className={"tile-value" + (errShare && errShare >= 1 ? " tone-text-error" : "")}>
            {errShare === null ? "–" : `${fmtRate(errShare, lang)} %`}
          </div>
          <div className="tile-sub muted">{t("stats.kpi.errRateSub", { n: fmtCompact(total5xx, lang), total: fmtCompact(totalResp, lang) })}</div>
        </div>
        <div className="tile">
          <div className="tile-label">{t("stats.kpi.traffic")}</div>
          <div className="tile-value">{bytesNow === null ? "–" : `${fmtBytes(bytesNow, lang)}/s`}</div>
          <div className="tile-sub muted">{t("stats.kpi.trafficSub", { v: fmtBytes(bytesTotal, lang) })}</div>
        </div>
      </div>

      <section className="card">
        <div className="card-head">
          <h2 className="card-title">{t("stats.chart.title")}</h2>
          <select className="select-sm" value={chartFe} onChange={(e) => setChartFe(e.target.value)} aria-label={t("stats.chart.scope")}>
            <option value="">{t("stats.chart.all")}</option>
            {frontends.map((f) => (
              <option key={f.proxy} value={f.proxy}>
                {f.proxy}
              </option>
            ))}
          </select>
        </div>
        <LineChart
          series={[
            { name: t("stats.chart.requests"), color: "var(--chart-1)", points: reqSeries, area: true },
            { name: t("stats.chart.errors"), color: "var(--chart-2)", points: errSeries },
          ]}
          format={(v) => fmtRate(v, lang)}
          timeFormat={timeFmt}
          label={t("stats.chart.label", { name: chartFe || t("stats.chart.all") })}
          emptyText={t("stats.chart.empty")}
        />
      </section>

      <div className="toolbar">
        <div className="search">
          <Icon name="search" />
          <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("stats.search")} aria-label={t("stats.search")} />
        </div>
        <div className="seg" role="group" aria-label={t("stats.filter")}>
          {(["all", "frontends", "backends"] as Filter[]).map((f) => (
            <button key={f} type="button" className={filter === f ? "active" : ""} aria-pressed={filter === f} onClick={() => setFilter(f)}>
              {t(("stats.filter." + f) as Key)}
            </button>
          ))}
        </div>
        <label className="checkbox">
          <input type="checkbox" checked={problemsOnly} onChange={(e) => setProblemsOnly(e.target.checked)} /> {t("stats.problemsOnly")}
        </label>
        <label className="checkbox">
          <input type="checkbox" checked={allCols} onChange={(e) => setAllCols(e.target.checked)} /> {t("stats.allColumns")}
        </label>
      </div>

      <CodeLegend />

      {showFe && (
        <section className="card stats-card">
          <h2 className="card-title">{t("stats.frontends")}</h2>
          {feRows.length === 0 ? <Empty>{t("stats.empty")}</Empty> : <StatTable rows={feRows} samples={samples} allCols={allCols} kind="frontend" />}
        </section>
      )}

      {showBe &&
        (backendGroups.length === 0 ? (
          <Empty>{t("stats.empty")}</Empty>
        ) : (
          backendGroups.map((g) => (
            <section key={g.name} className="card stats-card">
              <div className="card-head">
                <h2 className="card-title mono">{g.name}</h2>
                <span className="muted small">
                  {g.total.algo && <Badge>{g.total.algo}</Badge>}{" "}
                  <Link to={`/services?q=${encodeURIComponent(g.name)}`} className="btn-link">
                    {t("stats.toService")} <Icon name="chevron" />
                  </Link>
                </span>
              </div>
              <StatTable
                rows={[...g.servers.filter((s) => !problemsOnly || isProblem(s)), g.total]}
                samples={samples}
                allCols={allCols}
                kind="backend"
              />
            </section>
          ))
        ))}

      <p className="muted small">{t("stats.hint")}</p>
    </div>
  );
}

function CodeLegend() {
  const { t } = useT();
  const items: { key: keyof CodeCounts; tone: string }[] = [
    { key: "c2", tone: "var(--ok)" },
    { key: "c3", tone: "var(--neutral)" },
    { key: "c4", tone: "var(--warn)" },
    { key: "c5", tone: "var(--error)" },
  ];
  return (
    <div className="chart-legend" aria-label={t("stats.col.codes")}>
      <span className="muted small">{t("stats.col.codes")}:</span>
      {items.map((i) => (
        <span key={i.key} className="chart-legend-item">
          <span className="chart-swatch" style={{ background: i.tone }} />
          {t(("stats.code." + i.key) as Key)}
        </span>
      ))}
    </div>
  );
}

function StatTable({ rows, samples, allCols, kind }: { rows: Stat[]; samples: Sample[]; allCols: boolean; kind: "frontend" | "backend" }) {
  const { t, lang } = useT();
  const codeLabels = {
    c2: t("stats.code.c2"),
    c3: t("stats.code.c3"),
    c4: t("stats.code.c4"),
    c5: t("stats.code.c5"),
    other: t("stats.code.other"),
  };
  const recent = samples.slice(-(SPARK_POINTS + 1));
  return (
    <div className="table-wrap stats-wrap">
      <table className="stats-table">
        <thead>
          <tr>
            <th>{t("stats.col.name")}</th>
            <th>{t("stats.col.status")}</th>
            <th className="num">{t("stats.col.rate")}</th>
            <th className="num" title={t("stats.col.sessionsHint")}>
              {t("stats.col.sessions")}
            </th>
            <th className="num">{t("stats.col.total")}</th>
            <th>{t("stats.col.codes")}</th>
            <th className="num">{t("stats.col.errors")}</th>
            {kind === "backend" && <th className="num">{t("stats.col.rtime")}</th>}
            <th className="num">{t("stats.col.traffic")}</th>
            {allCols && (
              <>
                <th className="num">{t("stats.col.queue")}</th>
                <th className="num">{t("stats.col.limit")}</th>
                {kind === "backend" && <th className="num">{t("stats.col.ctime")}</th>}
                {kind === "backend" && <th className="num">{t("stats.col.ttime")}</th>}
                <th className="num">4xx</th>
                <th className="num">5xx</th>
                <th className="num">{t("stats.col.denied")}</th>
                {kind === "backend" && <th className="num">{t("stats.col.retries")}</th>}
                <th className="num">{t("stats.col.aborts")}</th>
                {kind === "backend" && <th>{t("stats.col.check")}</th>}
                <th className="num">{t("stats.col.lastChange")}</th>
                {kind === "backend" && <th className="num">{t("stats.col.downtime")}</th>}
                {kind === "backend" && <th className="num">{t("stats.col.weight")}</th>}
              </>
            )}
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => {
            const key = rowKey(s);
            const rate = currentRate(samples, key, trafficCounter);
            const series = rateSeries(recent, key, trafficCounter);
            const errs = errorCount(s);
            const isTotal = s.type === "backend";
            const name = s.type === "frontend" ? s.proxy : isTotal ? t("stats.backendTotal") : s.name;
            return (
              <tr key={key} className={isTotal ? "stats-total" : ""}>
                <th scope="row" className={isTotal ? "" : "mono"}>
                  {name}
                  {s.addr && <div className="muted small mono">{s.addr}</div>}
                </th>
                <td>
                  <StatusBadge status={s.status} check={s.checkDesc} />
                </td>
                <td className="num">
                  <span className="rate-cell">
                    <Sparkline points={series} label={t("stats.sparkLabel", { name, v: fmtRate(rate, lang) })} />
                    <span>{fmtRate(rate, lang)}</span>
                  </span>
                </td>
                <td className="num" title={t("stats.col.sessionsHint")}>
                  {fmtInt(s.sessions, lang)}
                  <span className="muted"> / {fmtInt(s.sessionsMax, lang)}</span>
                </td>
                <td className="num">{fmtCompact(trafficCounter(s), lang)}</td>
                <td className="codes-cell">
                  <CodeBar
                    counts={{ c2: s.http2xx, c3: s.http3xx, c4: s.http4xx, c5: s.http5xx, other: s.http1xx + s.httpOther }}
                    labels={codeLabels}
                    fmt={(n) => fmtInt(n, lang)}
                  />
                </td>
                <td className={"num" + (errs > 0 ? " tone-text-error" : "")}>{fmtCompact(errs, lang)}</td>
                {kind === "backend" && <td className="num">{s.rtime ? fmtMs(s.rtime, lang) : "–"}</td>}
                <td className="num nowrap">
                  {fmtBytes(s.bytesIn, lang)} <span className="muted">/</span> {fmtBytes(s.bytesOut, lang)}
                </td>
                {allCols && (
                  <>
                    <td className="num">
                      {fmtInt(s.queue, lang)}
                      <span className="muted"> / {fmtInt(s.queueMax, lang)}</span>
                    </td>
                    <td className="num">{s.sessionLimit ? fmtInt(s.sessionLimit, lang) : "–"}</td>
                    {kind === "backend" && <td className="num">{s.ctime ? fmtMs(s.ctime, lang) : "–"}</td>}
                    {kind === "backend" && <td className="num">{s.ttime ? fmtMs(s.ttime, lang) : "–"}</td>}
                    <td className="num">{fmtInt(s.http4xx, lang)}</td>
                    <td className={"num" + (s.http5xx > 0 ? " tone-text-error" : "")}>{fmtInt(s.http5xx, lang)}</td>
                    <td className="num">{fmtInt(s.deniedReq + s.deniedResp, lang)}</td>
                    {kind === "backend" && (
                      <td className="num">
                        {fmtInt(s.retries, lang)}
                        <span className="muted"> / {fmtInt(s.redispatch, lang)}</span>
                      </td>
                    )}
                    <td className="num">
                      {fmtInt(s.cliAbort, lang)}
                      <span className="muted"> / {fmtInt(s.srvAbort, lang)}</span>
                    </td>
                    {kind === "backend" && (
                      <td className="nowrap small">
                        {s.checkStatus ? (
                          <>
                            <span className="mono">{s.checkStatus}</span>
                            {s.checkDuration ? <span className="muted"> · {fmtMs(s.checkDuration, lang)}</span> : null}
                          </>
                        ) : (
                          "–"
                        )}
                      </td>
                    )}
                    <td className="num">{t("stats.ago", { v: fmtDuration(s.lastChange) })}</td>
                    {kind === "backend" && <td className="num">{s.downtime ? fmtDuration(s.downtime) : "–"}</td>}
                    {kind === "backend" && <td className="num">{s.type === "server" ? fmtInt(s.weight, lang) : "–"}</td>}
                  </>
                )}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
