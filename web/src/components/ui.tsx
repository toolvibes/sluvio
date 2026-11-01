import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { useT, type Key } from "../i18n";

// ---------------------------------------------------------------------------
// icons (inline SVG, 16px, currentColor)

const paths: Record<string, ReactNode> = {
  dashboard: <path d="M3 3h7v9H3zM14 3h7v5h-7zM14 12h7v9h-7zM3 16h7v5H3z" />,
  services: (
    <>
      <rect x="3" y="4" width="18" height="6" rx="1.5" />
      <rect x="3" y="14" width="18" height="6" rx="1.5" />
      <path d="M7 7h.01M7 17h.01" />
    </>
  ),
  routing: <path d="M4 6h6l4 6-4 6H4M14 12h6M17 9l3 3-3 3" />,
  editor: <path d="M8 6l-6 6 6 6M16 6l6 6-6 6M13 4l-2 16" />,
  history: (
    <>
      <path d="M3 12a9 9 0 1 0 3-6.7L3 8" />
      <path d="M3 3v5h5M12 7v5l3 3" />
    </>
  ),
  setup: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  edit: <path d="M4 20h4L19 9l-4-4L4 16zM14 6l4 4" />,
  trash: <path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" />,
  code: <path d="M8 6l-6 6 6 6M16 6l6 6-6 6" />,
  chevron: <path d="M9 6l6 6-6 6" />,
  close: <path d="M6 6l12 12M18 6L6 18" />,
  check: <path d="M5 12l5 5L20 7" />,
  warn: <path d="M12 3l10 18H2zM12 10v5M12 18h.01" />,
  error: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v6M12 16h.01" />
    </>
  ),
  info: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v6M12 8h.01" />
    </>
  ),
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
    </>
  ),
  moon: <path d="M21 13A9 9 0 1 1 11 3a7 7 0 0 0 10 10z" />,
  system: (
    <>
      <rect x="3" y="4" width="18" height="12" rx="1.5" />
      <path d="M8 20h8M12 16v4" />
    </>
  ),
  logout: <path d="M15 4h4v16h-4M10 8l-4 4 4 4M6 12h11" />,
  menu: <path d="M4 6h16M4 12h16M4 18h16" />,
  search: (
    <>
      <circle cx="11" cy="11" r="7" />
      <path d="M20 20l-4-4" />
    </>
  ),
  copy: (
    <>
      <rect x="8" y="8" width="12" height="12" rx="1.5" />
      <path d="M16 8V4H4v12h4" />
    </>
  ),
  pause: <path d="M8 5v14M16 5v14" />,
  play: <path d="M7 5l12 7-12 7z" />,
  drain: <path d="M12 3v12M7 10l5 5 5-5M5 21h14" />,
  restore: (
    <>
      <path d="M3 12a9 9 0 1 0 3-6.7L3 8" />
      <path d="M3 3v5h5" />
    </>
  ),
  external: <path d="M14 4h6v6M20 4l-9 9M18 14v6H4V6h6" />,
};

export function Icon({ name, size = 16, className }: { name: keyof typeof paths | string; size?: number; className?: string }) {
  return (
    <svg
      className={"icon " + (className ?? "")}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {paths[name]}
    </svg>
  );
}

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false" className="logo">
      <rect x="1" y="1" width="30" height="30" rx="7" fill="var(--accent)" />
      <rect x="7" y="6" width="3" height="20" rx="1" fill="var(--accent-contrast)" />
      <rect x="22" y="6" width="3" height="20" rx="1" fill="var(--accent-contrast)" />
      <rect x="10" y="8" width="12" height="8" rx="1" fill="var(--accent-contrast)" opacity="0.9" />
      <path d="M10 20.5c2 0 2-1.5 4-1.5s2 1.5 4 1.5 2-1.5 4-1.5" stroke="var(--accent-contrast)" strokeWidth="1.8" fill="none" strokeLinecap="round" />
      <path d="M10 24.5c2 0 2-1.5 4-1.5s2 1.5 4 1.5 2-1.5 4-1.5" stroke="var(--accent-contrast)" strokeWidth="1.8" fill="none" strokeLinecap="round" opacity="0.7" />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// dialog with focus trap

export function Dialog({
  title,
  onClose,
  children,
  footer,
  wide,
  labelledBy,
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
  labelledBy?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const { t } = useT();
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    const el = ref.current;
    const focusables = () =>
      Array.from(
        el?.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      ).filter((x) => x.offsetParent !== null);
    const first = el?.querySelector<HTMLElement>("[data-autofocus]") ?? focusables()[1] ?? focusables()[0];
    first?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        closeRef.current();
      } else if (e.key === "Tab") {
        const f = focusables();
        if (f.length === 0) return;
        const a = f[0];
        const b = f[f.length - 1];
        if (e.shiftKey && document.activeElement === a) {
          e.preventDefault();
          b.focus();
        } else if (!e.shiftKey && document.activeElement === b) {
          e.preventDefault();
          a.focus();
        }
      }
    };
    el?.addEventListener("keydown", onKey);
    document.body.classList.add("modal-open");
    return () => {
      el?.removeEventListener("keydown", onKey);
      document.body.classList.remove("modal-open");
      prev?.focus?.();
    };
  }, []);
  return createPortal(
    <div className="backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div
        className={"dialog" + (wide ? " dialog-wide" : "")}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy ?? titleId}
        ref={ref}
      >
        <header className="dialog-head">
          <h2 id={titleId}>{title}</h2>
          <button type="button" className="btn-icon" onClick={onClose} aria-label={t("common.close")}>
            <Icon name="close" />
          </button>
        </header>
        <div className="dialog-body">{children}</div>
        {footer && <footer className="dialog-foot">{footer}</footer>}
      </div>
    </div>,
    document.body,
  );
}

// ---------------------------------------------------------------------------
// confirm dialog

export function Confirm({
  title,
  body,
  confirmLabel,
  danger,
  onConfirm,
  onClose,
}: {
  title: string;
  body: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useT();
  return (
    <Dialog
      title={title}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button
            type="button"
            className={danger ? "btn btn-danger" : "btn btn-primary"}
            data-autofocus
            onClick={() => {
              onConfirm();
              onClose();
            }}
          >
            {confirmLabel}
          </button>
        </>
      }
    >
      {body}
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// switch

export function Switch({
  checked,
  onChange,
  label,
  disabled,
  title,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      title={title ?? label}
      disabled={disabled}
      className={"switch" + (checked ? " on" : "")}
      onClick={() => onChange(!checked)}
    >
      <span className="switch-knob" />
    </button>
  );
}

// ---------------------------------------------------------------------------
// status badges

export type Tone = "ok" | "warn" | "error" | "maint" | "neutral" | "accent";

export function statusTone(status: string | undefined): Tone {
  if (!status) return "neutral";
  const s = status.toUpperCase();
  if (s.startsWith("MAINT")) return "maint";
  if (s.startsWith("DRAIN") || s.startsWith("NOLB")) return "warn";
  if (s.startsWith("UP ") || s.startsWith("UP(")) return "warn"; // going down
  if (s === "UP" || s === "OPEN") return "ok";
  if (s.startsWith("DOWN")) return "error";
  return "neutral";
}

export function StatusBadge({ status, title, check }: { status?: string; title?: string; check?: string }) {
  const { t } = useT();
  if (check && check.includes("INI") && status?.toUpperCase().startsWith("UP ")) {
    return (
      <span className="badge tone-neutral" title={title ?? status}>
        <span className="dot" aria-hidden="true" />
        {t("status.init")}
      </span>
    );
  }
  const tone = statusTone(status);
  let label: string;
  const s = (status ?? "").toUpperCase();
  if (!status) label = t("status.unknown");
  else if (s === "UP") label = t("status.up");
  else if (s.startsWith("UP ")) label = t("status.goingDown", { detail: status.slice(3) });
  else if (s === "DOWN") label = t("status.down");
  else if (s.startsWith("DOWN ")) label = t("status.goingUp", { detail: status.slice(5) });
  else if (s.startsWith("MAINT")) label = t("status.maint");
  else if (s.startsWith("DRAIN")) label = t("status.drain");
  else if (s === "NO CHECK") label = t("status.nocheck");
  else if (s === "OPEN") label = t("status.open");
  else label = status;
  return (
    <span className={"badge tone-" + tone} title={title ?? status}>
      <span className="dot" aria-hidden="true" />
      {label}
    </span>
  );
}

export function Badge({ tone = "neutral", children, title }: { tone?: Tone; children: ReactNode; title?: string }) {
  return (
    <span className={"badge tone-" + tone} title={title}>
      {children}
    </span>
  );
}

// ---------------------------------------------------------------------------
// form helpers

export function Field({
  label,
  hint,
  error,
  children,
  htmlFor,
  className,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  children: ReactNode;
  htmlFor?: string;
  className?: string;
}) {
  return (
    <div className={"field " + (className ?? "") + (error ? " has-error" : "")}>
      <label htmlFor={htmlFor}>{label}</label>
      {children}
      {error ? <div className="field-error">{error}</div> : hint ? <div className="field-hint">{hint}</div> : null}
    </div>
  );
}

export function TextField({
  label,
  hint,
  error,
  className,
  ...props
}: { label: ReactNode; hint?: ReactNode; error?: string; className?: string } & InputHTMLAttributes<HTMLInputElement>) {
  const id = useId();
  return (
    <Field label={label} hint={hint} error={error} htmlFor={id} className={className}>
      <input id={id} aria-invalid={!!error} {...props} />
    </Field>
  );
}

/** Edits a list of short values (hosts, paths) as chips. */
export function ListInput({
  label,
  values,
  onChange,
  placeholder,
  validate,
  hint,
  mono,
}: {
  label: ReactNode;
  values: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
  validate?: (v: string) => string | null;
  hint?: ReactNode;
  mono?: boolean;
}) {
  const [draft, setDraft] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const id = useId();
  const { t } = useT();
  const add = () => {
    const parts = draft
      .split(/[\s,]+/)
      .map((x) => x.trim())
      .filter(Boolean);
    if (parts.length === 0) return;
    for (const p of parts) {
      const e = validate?.(p);
      if (e) {
        setErr(e);
        return;
      }
    }
    onChange(Array.from(new Set([...values, ...parts])));
    setDraft("");
    setErr(null);
  };
  return (
    <Field label={label} hint={hint} error={err ?? undefined} htmlFor={id}>
      <div className={"chips" + (mono ? " mono" : "")}>
        {values.map((v) => (
          <span className="chip" key={v}>
            {v}
            <button type="button" onClick={() => onChange(values.filter((x) => x !== v))} aria-label={t("common.removeItem", { item: v })}>
              <Icon name="close" size={12} />
            </button>
          </span>
        ))}
        <input
          id={id}
          value={draft}
          placeholder={placeholder}
          onChange={(e) => {
            setDraft(e.target.value);
            setErr(null);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === ",") {
              e.preventDefault();
              add();
            } else if (e.key === "Backspace" && draft === "" && values.length > 0) {
              onChange(values.slice(0, -1));
            }
          }}
          onBlur={add}
        />
      </div>
    </Field>
  );
}

// ---------------------------------------------------------------------------
// toasts

export interface Toast {
  id: number;
  tone: "ok" | "warn" | "error" | "info";
  title: string;
  body?: string;
  details?: string;
}

const ToastCtx = createContext<(t: Omit<Toast, "id">) => void>(() => {});

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const { t } = useT();
  const push = useCallback((x: Omit<Toast, "id">) => {
    const id = Date.now() + Math.random();
    setToasts((l) => [...l.slice(-3), { ...x, id }]);
    if (x.tone !== "error") window.setTimeout(() => setToasts((l) => l.filter((y) => y.id !== id)), x.details ? 12000 : 6000);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((x) => (
          <div key={x.id} className={"toast toast-" + x.tone}>
            <Icon name={x.tone === "ok" ? "check" : x.tone === "info" ? "info" : x.tone === "warn" ? "warn" : "error"} />
            <div className="toast-text">
              <strong>{x.title}</strong>
              {x.body && <div>{x.body}</div>}
              {x.details && (
                <details>
                  <summary>{t("common.details")}</summary>
                  <pre>{x.details}</pre>
                </details>
              )}
            </div>
            <button type="button" className="btn-icon" onClick={() => setToasts((l) => l.filter((y) => y.id !== x.id))} aria-label={t("common.close")}>
              <Icon name="close" size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast() {
  return useContext(ToastCtx);
}

// ---------------------------------------------------------------------------
// misc

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="spinner-wrap" role="status">
      <span className="spinner" aria-hidden="true" />
      {label && <span>{label}</span>}
    </span>
  );
}

export function CopyBlock({ text, label }: { text: string; label?: string }) {
  const { t } = useT();
  const [copied, setCopied] = useState(false);
  return (
    <div className="copyblock">
      {label && <div className="copyblock-label">{label}</div>}
      <pre>{text}</pre>
      <button
        type="button"
        className="btn btn-sm"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(text);
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1500);
          } catch {
            /* clipboard unavailable */
          }
        }}
      >
        <Icon name={copied ? "check" : "copy"} size={14} />
        {copied ? t("common.copied") : t("common.copy")}
      </button>
    </div>
  );
}

export function kindKey(kind: string): Key {
  return ("kind." + kind) as Key;
}
