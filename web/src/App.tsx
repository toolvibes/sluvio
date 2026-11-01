import { useCallback, useEffect, useState } from "react";
import { createBrowserRouter, NavLink, Outlet, RouterProvider, useLocation } from "react-router-dom";
import { api, ApiError, type Me } from "./api";
import { useT, type Lang } from "./i18n";
import { AppProvider, useApp, useTheme, type ThemePref } from "./state";
import { Icon, Logo, Spinner, ToastProvider } from "./components/ui";
import { ReviewProvider } from "./components/review";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import Services from "./pages/Services";
import ServiceWizard from "./pages/ServiceWizard";
import Routing from "./pages/Routing";
import Stats from "./pages/Stats";
import Editor from "./pages/Editor";
import History from "./pages/History";
import Setup from "./pages/Setup";

const router = createBrowserRouter([
  {
    path: "/",
    element: <Shell />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: "services", element: <Services /> },
      { path: "services/new", element: <ServiceWizard /> },
      { path: "stats", element: <Stats /> },
      { path: "routing", element: <Routing /> },
      { path: "config", element: <Editor /> },
      { path: "history", element: <History /> },
      { path: "history/:id", element: <History /> },
      { path: "setup", element: <Setup /> },
      { path: "*", element: <Dashboard /> },
    ],
  },
]);

export default function App() {
  return (
    <ToastProvider>
      <AuthGate />
    </ToastProvider>
  );
}

function AuthGate() {
  const [me, setMe] = useState<Me | null>(null);
  const [state, setState] = useState<"loading" | "login" | "ready" | "error">("loading");
  const [err, setErr] = useState<string>("");
  const { errorText } = useT();
  const check = useCallback(async () => {
    try {
      const m = await api.me();
      setMe(m);
      setState("ready");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) setState("login");
      else {
        setErr(errorText(e));
        setState("error");
      }
    }
  }, [errorText]);
  useEffect(() => {
    void check();
    const onUnauthorized = () => setState((s) => (s === "ready" ? "login" : s));
    window.addEventListener("sluiceops:unauthorized", onUnauthorized);
    return () => window.removeEventListener("sluiceops:unauthorized", onUnauthorized);
  }, [check]);
  if (state === "loading")
    return (
      <div className="center-screen">
        <Spinner />
      </div>
    );
  if (state === "error")
    return (
      <div className="center-screen">
        <div className="notice notice-error">{err}</div>
        <button type="button" className="btn" onClick={() => void check()}>
          ↻
        </button>
      </div>
    );
  if (state === "login" || !me) return <Login onLoggedIn={check} />;
  return (
    <AppProvider me={me} onLogout={() => setState("login")}>
      <ReviewProvider>
        <RouterProvider router={router} />
      </ReviewProvider>
    </AppProvider>
  );
}

const nav = [
  { to: "/", icon: "dashboard", key: "nav.dashboard", end: true },
  { to: "/services", icon: "services", key: "nav.services" },
  { to: "/stats", icon: "stats", key: "nav.stats" },
  { to: "/routing", icon: "routing", key: "nav.routing" },
  { to: "/config", icon: "editor", key: "nav.config" },
  { to: "/history", icon: "history", key: "nav.history" },
  { to: "/setup", icon: "setup", key: "nav.setup" },
] as const;

function Shell() {
  const { t } = useT();
  const { readOnly, me, logout, configError } = useApp();
  const [menuOpen, setMenuOpen] = useState(false);
  const loc = useLocation();
  useEffect(() => setMenuOpen(false), [loc.pathname]);
  const { errorText } = useT();
  return (
    <div className="shell">
      <a href="#main" className="skip-link">
        {t("app.skip")}
      </a>
      <header className="topbar">
        <button
          type="button"
          className="btn-icon menu-btn"
          aria-label={t("app.menu")}
          aria-expanded={menuOpen}
          aria-controls="sidenav"
          onClick={() => setMenuOpen((x) => !x)}
        >
          <Icon name="menu" size={20} />
        </button>
        <NavLink to="/" className="brand">
          <Logo />
          <span>SluiceOps</span>
        </NavLink>
        <StatusPill />
        <div className="topbar-spacer" />
        <LangSwitch />
        <ThemeSwitch />
        <div className="user">
          <span className="user-name" title={me.user}>
            {me.user}
          </span>
          {!me.authDisabled && (
            <button type="button" className="btn-icon" onClick={() => void logout()} aria-label={t("app.logout")} title={t("app.logout")}>
              <Icon name="logout" />
            </button>
          )}
        </div>
      </header>
      <nav id="sidenav" className={"sidenav" + (menuOpen ? " open" : "")} aria-label={t("app.navigation")}>
        {nav.map((n) => (
          <NavLink key={n.to} to={n.to} end={"end" in n ? n.end : false} className={({ isActive }) => "navlink" + (isActive ? " active" : "")}>
            <Icon name={n.icon} />
            <span>{t(n.key)}</span>
          </NavLink>
        ))}
        <div className="sidenav-foot muted">v{me.version}</div>
      </nav>
      <main id="main" className="main" tabIndex={-1}>
        {readOnly && (
          <div className="notice notice-warn banner">
            <Icon name="warn" /> {t("app.readOnlyBanner")}
          </div>
        )}
        {configError != null && (
          <div className="notice notice-error banner" role="alert">
            <Icon name="error" /> {t("app.configError")}: {errorText(configError)}
          </div>
        )}
        <Outlet />
      </main>
    </div>
  );
}

function StatusPill() {
  const { status } = useApp();
  const { t } = useT();
  if (!status) return null;
  if (!status.available)
    return (
      <NavLink to="/setup" className="pill tone-neutral" title={status.error}>
        <span className="dot" aria-hidden="true" />
        {t("app.noLiveStatus")}
      </NavLink>
    );
  const down = status.stats.filter((s) => s.type === "server" && s.status.toUpperCase().startsWith("DOWN")).length;
  return (
    <span className={"pill " + (down ? "tone-warn" : "tone-ok")} title={status.info ? `PID ${status.info.pid}` : undefined}>
      <span className="dot" aria-hidden="true" />
      <span className="pill-text">
        <span className="pill-name">HAProxy </span>
        {status.info?.version.split("-")[0]}
        <span className="pill-extra"> · {down ? t("app.serversDown", { n: down }) : t("app.running")}</span>
      </span>
    </span>
  );
}

function LangSwitch() {
  const { lang, setLang, t } = useT();
  return (
    <div className="seg" role="group" aria-label={t("app.language")}>
      {(["de", "en"] as Lang[]).map((l) => (
        <button key={l} type="button" aria-pressed={lang === l} className={lang === l ? "active" : ""} onClick={() => setLang(l)}>
          {l.toUpperCase()}
        </button>
      ))}
    </div>
  );
}

function ThemeSwitch() {
  const [pref, setPref] = useTheme();
  const { t } = useT();
  const order: ThemePref[] = ["system", "light", "dark"];
  const next = order[(order.indexOf(pref) + 1) % order.length];
  const label = t(("theme." + pref) as "theme.system");
  return (
    <button
      type="button"
      className="btn-icon"
      onClick={() => setPref(next)}
      aria-label={t("theme.toggle", { current: label })}
      title={t("theme.toggle", { current: label })}
    >
      <Icon name={pref === "system" ? "system" : pref === "light" ? "sun" : "moon"} />
    </button>
  );
}
