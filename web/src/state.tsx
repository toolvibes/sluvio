import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { api, type ConfigResponse, type Me, type Status, type SystemInfo } from "./api";

interface AppState {
  me: Me;
  config: ConfigResponse | null;
  configError: unknown;
  status: Status | null;
  system: SystemInfo | null;
  readOnly: boolean;
  refreshConfig: () => Promise<void>;
  refreshStatus: () => Promise<void>;
  refreshSystem: () => Promise<void>;
  refreshAll: () => Promise<void>;
  logout: () => Promise<void>;
}

const Ctx = createContext<AppState | null>(null);

export function AppProvider({ me, onLogout, children }: { me: Me; onLogout: () => void; children: ReactNode }) {
  const [config, setConfig] = useState<ConfigResponse | null>(null);
  const [configError, setConfigError] = useState<unknown>(null);
  const [status, setStatus] = useState<Status | null>(null);
  const [system, setSystem] = useState<SystemInfo | null>(null);
  const alive = useRef(true);

  const refreshConfig = useCallback(async () => {
    try {
      const c = await api.config();
      if (alive.current) {
        setConfig(c);
        setConfigError(null);
      }
    } catch (e) {
      if (alive.current) setConfigError(e);
    }
  }, []);
  const refreshStatus = useCallback(async () => {
    try {
      const s = await api.status();
      if (alive.current) setStatus(s);
    } catch (e) {
      if (alive.current) setStatus({ available: false, error: String((e as Error).message ?? e), stats: [] });
    }
  }, []);
  const refreshSystem = useCallback(async () => {
    try {
      const s = await api.system();
      if (alive.current) setSystem(s);
    } catch {
      /* shown on setup page */
    }
  }, []);
  const refreshAll = useCallback(async () => {
    await Promise.all([refreshConfig(), refreshStatus(), refreshSystem()]);
  }, [refreshConfig, refreshStatus, refreshSystem]);

  useEffect(() => {
    alive.current = true;
    void refreshAll();
    return () => {
      alive.current = false;
    };
  }, [refreshAll]);

  // Poll live status every 5 s while the tab is visible.
  useEffect(() => {
    let timer: number | undefined;
    const tick = () => {
      if (document.visibilityState === "visible") void refreshStatus();
    };
    timer = window.setInterval(tick, 5000);
    const onVis = () => {
      if (document.visibilityState === "visible") {
        void refreshStatus();
        void refreshConfig();
      }
    };
    document.addEventListener("visibilitychange", onVis);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, [refreshStatus, refreshConfig]);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      onLogout();
    }
  }, [onLogout]);

  const readOnly = me.readOnly || !!config?.readOnly;
  return (
    <Ctx.Provider
      value={{ me, config, configError, status, system, readOnly, refreshConfig, refreshStatus, refreshSystem, refreshAll, logout }}
    >
      {children}
    </Ctx.Provider>
  );
}

export function useApp(): AppState {
  const c = useContext(Ctx);
  if (!c) throw new Error("useApp outside AppProvider");
  return c;
}

// ---------------------------------------------------------------------------
// theme

export type ThemePref = "system" | "light" | "dark";

export function useTheme(): [ThemePref, (t: ThemePref) => void] {
  const [pref, setPref] = useState<ThemePref>(() => {
    try {
      const v = localStorage.getItem("sluiceops.theme");
      if (v === "light" || v === "dark") return v;
    } catch {
      /* ignore */
    }
    return "system";
  });
  useEffect(() => {
    const root = document.documentElement;
    if (pref === "system") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", pref);
    try {
      if (pref === "system") localStorage.removeItem("sluiceops.theme");
      else localStorage.setItem("sluiceops.theme", pref);
    } catch {
      /* ignore */
    }
  }, [pref]);
  return [pref, setPref];
}
