import type { Stat } from "./api";
import type { Sample } from "./state";

/** Key of a stats row. */
export const rowKey = (s: Pick<Stat, "proxy" | "name">) => `${s.proxy}/${s.name}`;

/** The counter that best describes "traffic" of a row: HTTP requests where
 * HAProxy counts them (frontends, backends), otherwise sessions (servers, TCP). */
export function trafficCounter(s: Stat): number {
  return s.reqTotal > 0 ? s.reqTotal : s.total;
}

function find(sample: Sample, key: string): Stat | undefined {
  return sample.status.stats.find((x) => rowKey(x) === key);
}

/** Per-second rate of a counter between two samples, or null if unknown
 * (not enough data, or the counter was reset by a reload). */
export function rateBetween(a: Sample, b: Sample, key: string, pick: (s: Stat) => number): number | null {
  const sa = find(a, key);
  const sb = find(b, key);
  if (!sa || !sb) return null;
  const dt = (b.t - a.t) / 1000;
  if (dt <= 0) return null;
  const dv = pick(sb) - pick(sa);
  if (dv < 0) return null;
  return dv / dt;
}

/** Rate over the last two samples. */
export function currentRate(samples: Sample[], key: string, pick: (s: Stat) => number): number | null {
  if (samples.length < 2) return null;
  return rateBetween(samples[samples.length - 2], samples[samples.length - 1], key, pick);
}

export interface Point {
  t: number;
  v: number | null;
}

/** Time series of a per-second rate for one row. */
export function rateSeries(samples: Sample[], key: string, pick: (s: Stat) => number): Point[] {
  const out: Point[] = [];
  for (let i = 1; i < samples.length; i++) {
    out.push({ t: samples[i].t, v: rateBetween(samples[i - 1], samples[i], key, pick) });
  }
  return out;
}

/** Sum of a per-second rate over several rows (e.g. all frontends). */
export function sumRateSeries(samples: Sample[], keys: string[], pick: (s: Stat) => number): Point[] {
  const out: Point[] = [];
  for (let i = 1; i < samples.length; i++) {
    let sum = 0;
    let any = false;
    for (const k of keys) {
      const r = rateBetween(samples[i - 1], samples[i], k, pick);
      if (r !== null) {
        sum += r;
        any = true;
      }
    }
    out.push({ t: samples[i].t, v: any ? sum : null });
  }
  return out;
}

/** Number of HTTP responses with a status code (all classes). */
export function responses(s: Stat): number {
  return s.http1xx + s.http2xx + s.http3xx + s.http4xx + s.http5xx + s.httpOther;
}

/** Sum of the error counters HAProxy keeps for a row. */
export function errorCount(s: Stat): number {
  return s.errReq + s.errConn + s.errResp;
}

/** Is a server/backend row in a state that needs attention? */
export function isProblem(s: Stat): boolean {
  const st = s.status.toUpperCase();
  if (s.checkStatus?.includes("INI")) return false;
  if (s.type === "frontend") return st !== "OPEN";
  return st.startsWith("DOWN") || st.startsWith("UP ") || st === "NOLB";
}

// ---------------------------------------------------------------------------
// formatting

export function fmtCompact(n: number | null | undefined, lang: string): string {
  if (n === null || n === undefined || !isFinite(n)) return "–";
  return new Intl.NumberFormat(lang, { notation: "compact", maximumFractionDigits: n < 1000 ? 0 : 1 }).format(n);
}

export function fmtRate(n: number | null | undefined, lang: string): string {
  if (n === null || n === undefined || !isFinite(n)) return "–";
  const digits = n < 10 && n > 0 ? 1 : 0;
  return new Intl.NumberFormat(lang, { maximumFractionDigits: digits, minimumFractionDigits: 0 }).format(n);
}

export function fmtInt(n: number | null | undefined, lang: string): string {
  if (n === null || n === undefined || !isFinite(n)) return "–";
  return new Intl.NumberFormat(lang).format(n);
}

export function fmtBytes(n: number | null | undefined, lang: string): string {
  if (n === null || n === undefined || !isFinite(n)) return "–";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let u = 0;
  let v = n;
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024;
    u++;
  }
  return new Intl.NumberFormat(lang, { maximumFractionDigits: v < 10 && u > 0 ? 1 : 0 }).format(v) + " " + units[u];
}

export function fmtMs(n: number | null | undefined, lang: string): string {
  if (n === null || n === undefined || !isFinite(n)) return "–";
  if (n >= 10000) return new Intl.NumberFormat(lang, { maximumFractionDigits: 1 }).format(n / 1000) + " s";
  return new Intl.NumberFormat(lang).format(n) + " ms";
}

/** Seconds as a short duration: 45s, 12m, 3h 5m, 2d 4h. */
export function fmtDuration(sec: number | null | undefined): string {
  if (sec === null || sec === undefined || sec < 0 || !isFinite(sec)) return "–";
  if (sec < 60) return `${Math.round(sec)}s`;
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  const d = Math.floor(h / 24);
  return `${d}d ${h % 24}h`;
}
