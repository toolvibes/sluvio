import { useState, type FormEvent } from "react";
import { api } from "../api";
import { useT, type Lang } from "../i18n";
import { Icon, Logo, Spinner } from "../components/ui";

export default function Login({ onLoggedIn }: { onLoggedIn: () => void }) {
  const { t, errorText, lang, setLang } = useT();
  const [user, setUser] = useState("admin");
  const [pw, setPw] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      await api.login(user, pw);
      onLoggedIn();
    } catch (e2) {
      setErr(errorText(e2));
      setBusy(false);
    }
  };
  return (
    <div className="login-screen">
      <form className="login-card" onSubmit={submit}>
        <div className="login-brand">
          <Logo size={40} />
          <div>
            <h1>SluiceOps</h1>
            <p className="muted">{t("login.tagline")}</p>
          </div>
        </div>
        {err && (
          <div className="notice notice-error" role="alert">
            <Icon name="error" /> {err}
          </div>
        )}
        <div className="field">
          <label htmlFor="login-user">{t("login.user")}</label>
          <input id="login-user" autoComplete="username" value={user} onChange={(e) => setUser(e.target.value)} required />
        </div>
        <div className="field">
          <label htmlFor="login-pw">{t("login.password")}</label>
          <input id="login-pw" type="password" autoComplete="current-password" value={pw} onChange={(e) => setPw(e.target.value)} required autoFocus />
        </div>
        <button className="btn btn-primary btn-block" type="submit" disabled={busy}>
          {busy ? <Spinner /> : null}
          {t("login.submit")}
        </button>
        <p className="muted small">{t("login.hint")}</p>
        <div className="seg login-lang" role="group" aria-label={t("app.language")}>
          {(["de", "en"] as Lang[]).map((l) => (
            <button key={l} type="button" aria-pressed={lang === l} className={lang === l ? "active" : ""} onClick={() => setLang(l)}>
              {l === "de" ? "Deutsch" : "English"}
            </button>
          ))}
        </div>
      </form>
    </div>
  );
}
