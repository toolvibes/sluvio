import { backendStatOf, serverStatOf, type Backend, type Frontend, type Model, type Rule, type Stat, type Status } from "./api";

export interface RouteRef {
  frontend: Frontend;
  rule?: Rule; // undefined for default_backend usage
  isDefault: boolean;
  enabled: boolean;
}

export interface ServiceView {
  backend: Backend;
  routes: RouteRef[];
  stat?: Stat;
  up: number;
  down: number;
  maint: number;
  total: number;
  problem: boolean;
}

export function routesTo(model: Model, backend: string): RouteRef[] {
  const out: RouteRef[] = [];
  for (const f of model.frontends) {
    for (const r of f.rules) {
      if (r.backend === backend) out.push({ frontend: f, rule: r, isDefault: false, enabled: r.enabled && f.enabled });
    }
    if (f.defaultBackend === backend) out.push({ frontend: f, isDefault: true, enabled: f.enabled });
  }
  return out;
}

export function serverState(stats: Stat[] | undefined, backend: string, server: string): string | undefined {
  return serverStatOf(stats, backend, server)?.status;
}

export function services(model: Model, status: Status | null): ServiceView[] {
  const stats = status?.available ? status.stats : undefined;
  return model.backends.map((b) => {
    let up = 0,
      down = 0,
      maint = 0;
    for (const s of b.servers) {
      if (!s.enabled) continue;
      const st = (serverState(stats, b.name, s.name) ?? "").toUpperCase();
      if (st.startsWith("MAINT") || (!st && s.maintenance)) maint++;
      else if (st.startsWith("DOWN")) down++;
      else if (st.startsWith("UP") || st === "NO CHECK" || st.startsWith("DRAIN")) up++;
    }
    const total = b.servers.filter((s) => s.enabled).length;
    const stat = backendStatOf(stats, b.name);
    const problem = b.enabled && !!stats && total > 0 && (down > 0 || up === 0);
    return { backend: b, routes: routesTo(model, b.name), stat, up, down, maint, total, problem };
  });
}

export function slug(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/** Guesses the backend naming style used in the config ("backend-x", "be_x", …). */
export function backendPrefix(model: Model | undefined): string {
  if (!model || model.backends.length === 0) return "backend-";
  const counts: Record<string, number> = {};
  for (const b of model.backends) {
    const m = /^(backend-|backend_|be_|be-|bk_|bk-)/.exec(b.name);
    const p = m ? m[1] : "";
    counts[p] = (counts[p] ?? 0) + 1;
  }
  return Object.entries(counts).sort((a, b) => b[1] - a[1])[0][0];
}

export function fmtBytes(n: number): string {
  if (n < 1024) return n + " B";
  const u = ["KiB", "MiB", "GiB", "TiB"];
  let v = n;
  let i = -1;
  do {
    v /= 1024;
    i++;
  } while (v >= 1024 && i < u.length - 1);
  return v.toFixed(v < 10 ? 1 : 0) + " " + u[i];
}
