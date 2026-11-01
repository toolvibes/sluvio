import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { api, ApiError, type ApplyResult, type DiffResult, type Op, type Preview, type Validation } from "../api";
import { useT } from "../i18n";
import { useApp } from "../state";
import { Badge, Dialog, Icon, Spinner, useToast } from "./ui";

// ---------------------------------------------------------------------------
// diff view

export function DiffView({ diff, maxHeight }: { diff: DiffResult; maxHeight?: number }) {
  const { t } = useT();
  if (diff.hunks.length === 0) return <div className="muted">{t("review.noDiff")}</div>;
  return (
    <div className="diff" style={maxHeight ? { maxHeight } : undefined} role="region" aria-label={t("review.diff")} tabIndex={0}>
      {diff.hunks.map((h, k) => (
        <div className="diff-hunk" key={k}>
          {k > 0 && <div className="diff-sep">⋯</div>}
          {h.lines.map((l, i) => (
            <div key={i} className={"diff-line " + (l.op === "+" ? "add" : l.op === "-" ? "del" : "ctx")}>
              <span className="ln">{l.old ?? ""}</span>
              <span className="ln">{l.new ?? ""}</span>
              <span className="sign" aria-hidden="true">
                {l.op === "=" ? " " : l.op === "+" ? "+" : "−"}
              </span>
              <span className="sr-only">{l.op === "+" ? t("review.added") : l.op === "-" ? t("review.removed") : ""}</span>
              <code>{l.text || " "}</code>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

export function ValidationView({ v }: { v: Validation }) {
  const { t } = useT();
  const problems = v.messages.filter((m) => m.level === "alert" || m.level === "warning");
  return (
    <div className={"validation " + (v.valid ? (problems.length ? "warn" : "ok") : "error")}>
      <div className="validation-head">
        <Icon name={v.valid ? (problems.length ? "warn" : "check") : "error"} />
        <strong>
          {v.skipped ? t("review.validationSkipped") : v.valid ? (problems.length ? t("review.validWithWarnings") : t("review.valid")) : t("review.invalid")}
        </strong>
        {v.duration && <span className="muted">· {v.duration}</span>}
      </div>
      {problems.length > 0 && (
        <ul className="validation-list">
          {problems.map((m, i) => (
            <li key={i} className={"lvl-" + m.level}>
              {m.line ? <span className="mono">{t("review.line", { n: m.line })}</span> : null} {m.message}
            </li>
          ))}
        </ul>
      )}
      {!v.valid && problems.length === 0 && v.output && <pre className="validation-output">{v.output}</pre>}
    </div>
  );
}

// ---------------------------------------------------------------------------
// review & apply dialog

export interface ReviewOptions {
  title: string;
  ops: Op[];
  /** Compact mode: summary first, diff collapsed (used for toggles). */
  compact?: boolean;
  version?: string;
  defaultMessage?: string;
}

type Review = (o: ReviewOptions) => Promise<ApplyResult | null>;

const Ctx = createContext<Review>(async () => null);

export function useReview(): Review {
  return useContext(Ctx);
}

interface Pending extends ReviewOptions {
  id: number;
  resolve: (r: ApplyResult | null) => void;
}

let reviewSeq = 0;

export function ReviewProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<Pending | null>(null);
  const review = useCallback<Review>(
    (o) =>
      new Promise((resolve) => {
        setPending({ ...o, id: ++reviewSeq, resolve });
      }),
    [],
  );
  return (
    <Ctx.Provider value={review}>
      {children}
      {pending && (
        <ReviewDialog
          key={pending.id}
          opts={pending}
          onDone={(r) => {
            pending.resolve(r);
            setPending(null);
          }}
        />
      )}
    </Ctx.Provider>
  );
}

function ReviewDialog({ opts, onDone }: { opts: Pending; onDone: (r: ApplyResult | null) => void }) {
  const { t, errorText } = useT();
  const { config, refreshAll, readOnly } = useApp();
  const toast = useToast();
  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [failedValidation, setFailedValidation] = useState<Validation | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState(opts.defaultMessage ?? "");
  const [showDiff, setShowDiff] = useState(!opts.compact);
  const versionRef = useRef(opts.version ?? config?.version ?? "");

  const load = useCallback(
    async (version: string) => {
      setPreview(null);
      setError(null);
      try {
        const p = await api.preview({ version, ops: opts.ops });
        setPreview(p);
      } catch (e) {
        if (e instanceof ApiError && e.status === 409) {
          // The file changed in the meantime: refresh and preview again.
          const fresh = await api.config();
          versionRef.current = fresh.version;
          setNotice(t("review.conflictReloaded"));
          void refreshAll();
          try {
            setPreview(await api.preview({ version: fresh.version, ops: opts.ops }));
          } catch (e2) {
            setError(e2);
          }
          return;
        }
        setError(e);
      }
    },
    [opts.ops, refreshAll, t],
  );

  useEffect(() => {
    void load(versionRef.current);
  }, [load]);

  const apply = async () => {
    if (!preview) return;
    setBusy(true);
    setError(null);
    try {
      const r = await api.apply({ version: preview.version, ops: opts.ops, message: message.trim() || opts.title });
      const actionErrors = (r.runtimeActions ?? []).filter((a) => a.error);
      if (r.reloadError) {
        toast({ tone: "warn", title: t("toast.savedReloadFailed"), body: r.reloadError, details: r.reloadOutput });
      } else if (actionErrors.length) {
        toast({ tone: "warn", title: t("toast.savedRuntimeFailed"), body: actionErrors.map((a) => `${a.backend}/${a.server}: ${a.error}`).join("\n") });
      } else if (r.reloaded) {
        toast({ tone: "ok", title: t("toast.appliedReload"), body: opts.title });
      } else if (!r.changed) {
        toast({ tone: "info", title: t("toast.noChange") });
      } else {
        toast({ tone: "ok", title: t("toast.appliedLive"), body: opts.title });
      }
      await refreshAll();
      onDone(r);
    } catch (e) {
      if (e instanceof ApiError && e.status === 422 && e.body.validation) {
        setFailedValidation(e.body.validation);
      } else if (e instanceof ApiError && e.status === 409) {
        setNotice(t("review.conflict"));
        const fresh = await api.config().catch(() => null);
        if (fresh) {
          versionRef.current = fresh.version;
          void refreshAll();
          void load(fresh.version);
        }
      } else {
        setError(e);
      }
    } finally {
      setBusy(false);
    }
  };

  const validation = failedValidation ?? preview?.validation;
  const canApply = !!preview && preview.changed && !!validation?.valid && !readOnly && !busy;
  const actions = preview?.runtimeActions ?? [];

  return (
    <Dialog
      title={opts.title}
      onClose={() => onDone(null)}
      wide={!opts.compact}
      footer={
        <>
          <button type="button" className="btn" onClick={() => onDone(null)}>
            {t("common.cancel")}
          </button>
          <button type="button" className="btn btn-primary" disabled={!canApply} onClick={apply} data-autofocus={opts.compact ? true : undefined}>
            {busy ? <Spinner /> : <Icon name="check" />}
            {preview && !preview.reloadRequired && actions.length > 0 ? t("review.applyLive") : t("review.apply")}
          </button>
        </>
      }
    >
      {notice && (
        <div className="notice notice-warn">
          <Icon name="warn" /> {notice}
        </div>
      )}
      {readOnly && (
        <div className="notice notice-warn">
          <Icon name="warn" /> {t("app.readOnlyBanner")}
        </div>
      )}
      {error != null && (
        <div className="notice notice-error" role="alert">
          <Icon name="error" /> {errorText(error)}
        </div>
      )}
      {!preview && error == null && <Spinner label={t("review.loading")} />}
      {preview && (
        <div className="review">
          <div className="review-summary">
            {preview.changed ? (
              <>
                <Badge tone="ok">+{preview.diff.added}</Badge>
                <Badge tone="error">−{preview.diff.removed}</Badge>
                <span>{t("review.lines")}</span>
              </>
            ) : (
              <span className="muted">{t("review.noChange")}</span>
            )}
          </div>
          {preview.changed && (
            <div className="review-activation">
              {preview.reloadRequired ? (
                <>
                  <Icon name="restore" />
                  <span>{t("review.willReload")}</span>
                </>
              ) : actions.length > 0 ? (
                <>
                  <Icon name="play" />
                  <div>
                    <div>{t("review.willApplyLive")}</div>
                    <ul className="plain">
                      {actions.map((a) => (
                        <li key={a.backend + a.server} className="mono">
                          {a.backend}/{a.server} → {t(("state." + a.state) as "state.ready")}
                        </li>
                      ))}
                    </ul>
                  </div>
                </>
              ) : (
                <>
                  <Icon name="info" />
                  <span>{t("review.noActivation")}</span>
                </>
              )}
            </div>
          )}
          {validation && preview.changed && <ValidationView v={validation} />}
          {preview.changed && (
            <div>
              {opts.compact && (
                <button type="button" className="btn-link" onClick={() => setShowDiff((x) => !x)} aria-expanded={showDiff}>
                  <Icon name="chevron" className={showDiff ? "rot90" : ""} /> {showDiff ? t("review.hideDiff") : t("review.showDiff")}
                </button>
              )}
              {showDiff && <DiffView diff={preview.diff} maxHeight={opts.compact ? 260 : 420} />}
            </div>
          )}
          {preview.changed && (
            <div className="field">
              <label htmlFor="review-msg">{t("review.message")}</label>
              <input
                id="review-msg"
                value={message}
                placeholder={opts.title}
                onChange={(e) => setMessage(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && canApply) void apply();
                }}
              />
            </div>
          )}
        </div>
      )}
    </Dialog>
  );
}
