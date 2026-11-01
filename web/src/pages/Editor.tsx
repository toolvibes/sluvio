import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useBlocker } from "react-router-dom";
import { api, ApiError, type Validation } from "../api";
import { useT } from "../i18n";
import { useApp } from "../state";
import { Confirm, Icon, Spinner } from "../components/ui";
import { useReview, ValidationView } from "../components/review";

const LINE_HEIGHT = 20;

export default function Editor() {
  const { t, errorText } = useT();
  const { readOnly, config } = useApp();
  const review = useReview();
  const [orig, setOrig] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [version, setVersion] = useState("");
  const [path, setPath] = useState("");
  const [err, setErr] = useState<unknown>(null);
  const [validation, setValidation] = useState<Validation | null>(null);
  const [validating, setValidating] = useState(false);
  const [externalChange, setExternalChange] = useState(false);
  const ta = useRef<HTMLTextAreaElement>(null);
  const escPressed = useRef(false);
  const leaving = useRef(false);
  const gutter = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    try {
      const r = await api.raw();
      setOrig(r.content);
      setText(r.content);
      setVersion(r.version);
      setPath(r.path);
      setValidation(null);
      setExternalChange(false);
    } catch (e) {
      setErr(e);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const dirty = orig !== null && text !== orig;
  // Notice when the file changed on disk while editing.
  useEffect(() => {
    if (!config || !version || config.version === version) return;
    if (dirty) setExternalChange(true);
    else void load();
  }, [config, version, dirty, load]);

  const blocker = useBlocker(({ currentLocation, nextLocation }) => dirty && currentLocation.pathname !== nextLocation.pathname);
  useEffect(() => {
    const h = (e: BeforeUnloadEvent) => {
      if (dirty) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", h);
    return () => window.removeEventListener("beforeunload", h);
  }, [dirty]);

  const lines = useMemo(() => text.split("\n").length, [text]);
  const errorLines = useMemo(() => {
    const s = new Map<number, string>();
    for (const m of validation?.messages ?? []) if (m.line && (m.level === "alert" || m.level === "warning")) s.set(m.line, s.get(m.line) === "alert" ? "alert" : m.level);
    return s;
  }, [validation]);

  const jump = (line: number) => {
    const el = ta.current;
    if (!el) return;
    const parts = text.split("\n");
    let pos = 0;
    for (let i = 0; i < line - 1 && i < parts.length; i++) pos += parts[i].length + 1;
    el.focus();
    el.setSelectionRange(pos, pos + (parts[line - 1]?.length ?? 0));
    el.scrollTop = Math.max(0, (line - 1) * LINE_HEIGHT - el.clientHeight / 3);
  };

  const validate = async () => {
    setValidating(true);
    try {
      const p = await api.preview({ version: "", ops: [{ op: "raw.replace", content: text }] });
      setValidation(p.validation);
      const first = p.validation.messages.find((m) => m.line && m.level === "alert");
      if (first?.line) jump(first.line);
    } catch (e) {
      if (e instanceof ApiError && e.body.validation) setValidation(e.body.validation);
      else setErr(e);
    } finally {
      setValidating(false);
    }
  };

  const save = async () => {
    const r = await review({ title: t("editor.saveTitle"), ops: [{ op: "raw.replace", content: text }], version });
    if (r) await load();
  };

  if (orig === null) return err ? <div className="notice notice-error">{errorText(err)}</div> : <Spinner label={t("common.loading")} />;

  return (
    <div className="page page-editor">
      <div className="page-head">
        <div>
          <h1>{t("editor.title")}</h1>
          <p className="muted mono small">{path}</p>
        </div>
        <div className="row-wrap">
          {dirty && <span className="badge tone-warn">{t("editor.unsaved")}</span>}
          <button type="button" className="btn" onClick={() => setText(orig)} disabled={!dirty}>
            {t("editor.discard")}
          </button>
          <button type="button" className="btn" onClick={validate} disabled={validating}>
            {validating ? <Spinner /> : <Icon name="check" />} {t("editor.validate")}
          </button>
          <button type="button" className="btn btn-primary" onClick={save} disabled={!dirty || readOnly}>
            {t("editor.save")}
          </button>
        </div>
      </div>
      {externalChange && (
        <div className="notice notice-warn">
          <Icon name="warn" /> {t("editor.externalChange")}{" "}
          <button type="button" className="btn btn-sm" onClick={() => void load()}>
            {t("editor.reload")}
          </button>
        </div>
      )}
      {validation && (
        <div className="editor-validation">
          <ValidationView v={validation} />
          {validation.messages.some((m) => m.line) && (
            <div className="row-wrap">
              {validation.messages
                .filter((m) => m.line && (m.level === "alert" || m.level === "warning"))
                .map((m, i) => (
                  <button key={i} type="button" className={"btn btn-sm " + (m.level === "alert" ? "btn-danger-ghost" : "")} onClick={() => jump(m.line!)}>
                    {t("editor.goto", { n: m.line! })}
                  </button>
                ))}
            </div>
          )}
        </div>
      )}
      <div className="code-editor">
        <div className="gutter" ref={gutter} aria-hidden="true">
          {Array.from({ length: lines }, (_, i) => (
            <div key={i} className={errorLines.has(i + 1) ? "gl " + (errorLines.get(i + 1) === "alert" ? "gl-error" : "gl-warn") : "gl"}>
              {i + 1}
            </div>
          ))}
        </div>
        <textarea
          ref={ta}
          value={text}
          readOnly={readOnly}
          spellCheck={false}
          autoCapitalize="off"
          autoComplete="off"
          wrap="off"
          aria-label={t("editor.title")}
          onChange={(e) => {
            setText(e.target.value);
          }}
          onScroll={(e) => {
            if (gutter.current) gutter.current.scrollTop = e.currentTarget.scrollTop;
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              escPressed.current = true;
              return;
            }
            const esc = escPressed.current;
            escPressed.current = false;
            if (e.key === "Tab" && !e.shiftKey && !esc) {
              // insert indentation instead of leaving the editor; Esc then Tab leaves
              e.preventDefault();
              const el = e.currentTarget;
              const s = el.selectionStart;
              const v = el.value;
              const next = v.slice(0, s) + "    " + v.slice(el.selectionEnd);
              setText(next);
              requestAnimationFrame(() => el.setSelectionRange(s + 4, s + 4));
            } else if ((e.ctrlKey || e.metaKey) && e.key === "s") {
              e.preventDefault();
              if (dirty && !readOnly) void save();
            }
          }}
        />
      </div>
      <p className="muted small">{t("editor.hint")}</p>
      {blocker.state === "blocked" && (
        <Confirm
          title={t("editor.leaveTitle")}
          body={t("editor.leaveBody")}
          confirmLabel={t("editor.leave")}
          danger
          onConfirm={() => {
            leaving.current = true;
            blocker.proceed?.();
          }}
          onClose={() => {
            if (!leaving.current) blocker.reset?.();
            leaving.current = false;
          }}
        />
      )}
    </div>
  );
}
