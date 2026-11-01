// Typed client for the SluiceOps HTTP API.

export interface Bind {
  line: number;
  address: string;
  params?: string[];
  ssl: boolean;
  enabled: boolean;
}

export interface ACL {
  line: number;
  name: string;
  criterion: string;
  flags?: string[];
  values: string[];
  enabled: boolean;
  kind: string;
}

export interface Rule {
  line: number;
  backend: string;
  cond?: string;
  condTest?: string;
  enabled: boolean;
  kind: "host" | "path" | "host+path" | "other" | "always";
  hosts?: string[];
  paths?: string[];
  text: string;
  dynamic?: boolean;
}

export interface Frontend {
  kind: string;
  name: string;
  line: number;
  enabled: boolean;
  description?: string;
  mode?: string;
  binds: Bind[];
  acls: ACL[];
  rules: Rule[];
  defaultBackend?: string;
  defaultLine?: number;
}

export interface HealthCheck {
  method?: string;
  path?: string;
  expect?: string;
}

export interface Usage {
  frontend: string;
  line: number;
  default: boolean;
  enabled: boolean;
}

export interface Server {
  line: number;
  name: string;
  address: string;
  port?: string;
  enabled: boolean;
  maintenance: boolean;
  check: boolean;
  inter?: string;
  fall?: string;
  rise?: string;
  weight?: string;
  backup: boolean;
  ssl: boolean;
  verify?: string;
  extra?: string[];
  text: string;
}

export interface Backend {
  kind: string;
  name: string;
  line: number;
  enabled: boolean;
  description?: string;
  mode?: string;
  balance?: string;
  healthCheck?: HealthCheck;
  timeouts?: Record<string, string>;
  servers: Server[];
  usedBy: Usage[];
}

export interface Generic {
  kind: string;
  name: string;
  line: number;
  enabled: boolean;
  description?: string;
}

export interface Model {
  frontends: Frontend[];
  backends: Backend[];
  other: Generic[] | null;
}

export interface ConfigResponse {
  version: string;
  path: string;
  modTime: string;
  readOnly: boolean;
  model: Model;
}

export interface ServerSpec {
  name: string;
  address: string;
  port: string;
  check: boolean;
  inter?: string;
  fall?: string;
  rise?: string;
  weight?: string;
  backup?: boolean;
  ssl?: boolean;
  verify?: string;
  maintenance?: boolean;
  extra?: string[];
}

export interface BackendSpec {
  name: string;
  description?: string;
  mode?: string;
  balance?: string;
  healthCheck?: HealthCheck;
  timeouts?: Record<string, string>;
  servers?: ServerSpec[];
}

export interface FrontendSpec {
  name: string;
  description?: string;
  mode?: string;
  binds: string[];
  defaultBackend?: string;
}

export interface RouteSpec {
  frontend: string;
  backend: string;
  hosts?: string[];
  paths?: string[];
}

export interface Op {
  op: string;
  frontend?: string;
  backend?: string;
  name?: string;
  kind?: string;
  line?: number;
  enabled?: boolean;
  keepBackend?: boolean;
  server?: ServerSpec;
  backendSpec?: BackendSpec;
  frontendSpec?: FrontendSpec;
  route?: RouteSpec;
  content?: string;
}

export interface DiffLine {
  op: "=" | "+" | "-";
  text: string;
  old?: number;
  new?: number;
}

export interface Hunk {
  oldStart: number;
  newStart: number;
  lines: DiffLine[];
}

export interface DiffResult {
  added: number;
  removed: number;
  hunks: Hunk[];
}

export interface ValidationMessage {
  level: string;
  line?: number;
  message: string;
}

export interface Validation {
  valid: boolean;
  skipped?: boolean;
  output: string;
  messages: ValidationMessage[];
  duration?: string;
}

export interface RuntimeAction {
  backend: string;
  server: string;
  state: string;
  error?: string;
}

export interface Preview {
  version: string;
  newVersion: string;
  diff: DiffResult;
  validation: Validation;
  reloadRequired: boolean;
  runtimeActions: RuntimeAction[] | null;
  changed: boolean;
}

export interface HistoryEntry {
  id: number;
  time: string;
  user?: string;
  message?: string;
  kind: string;
  ops?: string[];
  hash: string;
  size: number;
  reloaded: boolean;
  added: number;
  removed: number;
}

export interface ApplyResult extends Preview {
  entry?: HistoryEntry;
  reloaded: boolean;
  reloadOutput?: string;
  reloadError?: string;
}

export interface Info {
  version: string;
  uptime: string;
  uptimeSec: number;
  pid: string;
  currConns: string;
  maxConn: string;
  node: string;
}

export interface Stat {
  proxy: string;
  name: string;
  type: string;
  status: string;
  checkStatus?: string;
  checkDesc?: string;
  lastChange: number;
  sessions: number;
  rate: number;
  total: number;
  bytesIn: number;
  bytesOut: number;
  weight: number;
  http5xx: number;
  active: number;
  backup: number;
  addr?: string;
}

export interface Status {
  available: boolean;
  error?: string;
  info?: Info;
  stats: Stat[];
}

export interface Check {
  id: string;
  ok: boolean;
  level: "ok" | "warn" | "error";
  detail?: string;
  params?: Record<string, string>;
}

export interface SystemInfo {
  version: string;
  checks: Check[];
  settings: Record<string, string>;
  readOnly: boolean;
}

export interface Me {
  user: string;
  authDisabled: boolean;
  readOnly: boolean;
  version: string;
}

export interface ApiErrorBody {
  code: string;
  message: string;
  params?: Record<string, string>;
  validation?: Validation;
}

export class ApiError extends Error {
  status: number;
  body: ApiErrorBody;
  constructor(status: number, body: ApiErrorBody) {
    super(body.message);
    this.status = status;
    this.body = body;
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") {
    headers["X-SluiceOps"] = "1";
    headers["Content-Type"] = "application/json";
  }
  let res: Response;
  try {
    res = await fetch(url, {
      method,
      headers,
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (e) {
    throw new ApiError(0, { code: "network", message: String(e) });
  }
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!res.ok) {
    const err = (data as { error?: ApiErrorBody } | null)?.error ?? { code: "internal", message: text || res.statusText };
    if (res.status === 401) window.dispatchEvent(new Event("sluiceops:unauthorized"));
    throw new ApiError(res.status, err);
  }
  return data as T;
}

export interface ChangeRequest {
  version: string;
  ops: Op[];
  message?: string;
  reload?: "auto" | "never";
}

export const api = {
  me: () => request<Me>("GET", "/api/me"),
  login: (username: string, password: string) => request<{ user: string }>("POST", "/api/login", { username, password }),
  logout: () => request<{ ok: boolean }>("POST", "/api/logout", {}),
  config: () => request<ConfigResponse>("GET", "/api/config"),
  raw: () => request<{ version: string; content: string; path: string }>("GET", "/api/config/raw"),
  section: (kind: string, name: string) =>
    request<{ version: string; content: string }>(
      "GET",
      `/api/config/section?kind=${encodeURIComponent(kind)}&name=${encodeURIComponent(name)}`,
    ),
  preview: (req: ChangeRequest) => request<Preview>("POST", "/api/changes/preview", req),
  apply: (req: ChangeRequest) => request<ApplyResult>("POST", "/api/changes/apply", req),
  status: () => request<Status>("GET", "/api/status"),
  runtimeServer: (backend: string, server: string, state: "ready" | "drain" | "maint") =>
    request<{ ok: boolean }>("POST", "/api/runtime/server", { backend, server, state }),
  history: (limit = 100) => request<{ entries: HistoryEntry[] }>("GET", `/api/history?limit=${limit}`),
  historyEntry: (id: number) =>
    request<{ entry: HistoryEntry; content: string; diff: DiffResult; diffToCurrent?: DiffResult; current?: string }>(
      "GET",
      `/api/history/${id}`,
    ),
  rollback: (id: number, version: string) => request<ApplyResult>("POST", `/api/history/${id}/rollback`, { version }),
  system: () => request<SystemInfo>("GET", "/api/system"),
};

// Client side validation mirroring the backend rules.
export const NAME_RE = /^[A-Za-z0-9_.:-]+$/;
export const HOST_RE = /^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]+)?$/;
export const PORT_RE = /^[0-9+-]+$/;

export function serverStatOf(stats: Stat[] | undefined, backend: string, server: string): Stat | undefined {
  return stats?.find((s) => s.type === "server" && s.proxy === backend && s.name === server);
}

export function backendStatOf(stats: Stat[] | undefined, backend: string): Stat | undefined {
  return stats?.find((s) => s.type === "backend" && s.proxy === backend);
}

/** Converts a model server into a spec for editing. */
export function specOf(s: Server): ServerSpec {
  return {
    name: s.name,
    address: s.address,
    port: s.port ?? "",
    check: s.check,
    inter: s.inter ?? "",
    fall: s.fall ?? "",
    rise: s.rise ?? "",
    weight: s.weight ?? "",
    backup: s.backup,
    ssl: s.ssl,
    verify: s.verify ?? "",
    maintenance: s.maintenance,
    extra: s.extra ?? [],
  };
}
