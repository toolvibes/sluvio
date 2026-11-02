import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import de, { type Key } from "./de";
import en from "./en";
import { ApiError } from "../api";

export type Lang = "de" | "en";
const dicts = { de, en };

type Params = Record<string, string | number | undefined>;

interface I18n {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (key: Key, params?: Params) => string;
  /** Translates an API error into a readable message. */
  errorText: (e: unknown) => string;
  fmtTime: (iso: string) => string;
}

const Ctx = createContext<I18n | null>(null);

function initialLang(): Lang {
  try {
    const s = localStorage.getItem("sluvio.lang");
    if (s === "de" || s === "en") return s;
  } catch {
    /* storage unavailable */
  }
  return navigator.language?.toLowerCase().startsWith("de") ? "de" : "en";
}

function format(s: string, params?: Params): string {
  if (!params) return s;
  return s.replace(/\{(\w+)\}/g, (m, k) => (params[k] !== undefined ? String(params[k]) : m));
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initialLang);
  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);
  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem("sluvio.lang", l);
    } catch {
      /* ignore */
    }
  }, []);
  const value = useMemo<I18n>(() => {
    const dict = dicts[lang];
    const t = (key: Key, params?: Params) => format(dict[key] ?? de[key] ?? key, params);
    const errorText = (e: unknown) => {
      if (e instanceof ApiError) {
        const key = ("err." + e.body.code) as Key;
        if (key in dict) {
          const p: Params = { ...(e.body.params ?? {}) };
          if (p.kind) p.kind = kindLabel(String(p.kind), dict);
          return format(dict[key], p);
        }
        return e.body.message || dict["err.internal"];
      }
      return e instanceof Error ? e.message : String(e);
    };
    const fmtTime = (iso: string) => {
      const d = new Date(iso);
      if (isNaN(d.getTime())) return iso;
      return d.toLocaleString(lang === "de" ? "de-DE" : "en-GB", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
    };
    return { lang, setLang, t, errorText, fmtTime };
  }, [lang, setLang]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

function kindLabel(kind: string, dict: Record<Key, string>): string {
  const key = ("kind." + kind) as Key;
  return key in dict ? dict[key] : kind;
}

export function useT(): I18n {
  const c = useContext(Ctx);
  if (!c) throw new Error("useT outside I18nProvider");
  return c;
}

export type { Key };
