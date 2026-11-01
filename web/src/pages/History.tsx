import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type DiffResult, type HistoryEntry } from "../api";
import { useT, type Key } from "../i18n";
import { useApp } from "../state";
import { Badge, Dialog, Empty, Icon, Spinner, useToast } from "../components/ui";
import { DiffView } from "../components/review";
import { kindTone } from "./Dashboard";

export default function History() {
  const { t, fmtTime, errorText } = useT();
  const { config } = useApp();
  const { id } = useParams();
  const navigate = useNavigate();
  const [entries, setEntries] = useState<HistoryEntry[] | null>(null);
  const [err, setErr] = useState<unknown>(null);
  useEffect(() => {
    api
      .history(200)
      .then((r) => setEntries(r.entries))
      .catch(setErr);
  }, [config?.version]);
  const selected = id ? Number(id) : entries?.[0]?.id;
  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>{t("history.title")}</h1>
          <p className="muted">{t("history.subtitle")}</p>
        </div>
      </div>
      {err != null && <div className="notice notice-error">{errorText(err)}</div>}
      {entries === null && err == null && <Spinner label={t("common.loading")} />}
      {entries && entries.length === 0 && <Empty>{t("history.empty")}</Empty>}
      {entries && entries.length > 0 && (
        <div className="history">
          <ol className="history-list" aria-label={t("history.title")}>
            {entries.map((e, i) => (
              <li key={e.id}>
                <button
                  type="button"
                  className={"history-item" + (e.id === selected ? " active" : "")}
                  aria-current={e.id === selected ? "true" : undefined}
                  onClick={() => navigate(`/history/${e.id}`)}
                >
                  <div className="history-item-top">
                    <Badge tone={kindTone(e.kind)}>{t(("history.kind." + e.kind) as Key)}</Badge>
                    <span className="mono small">#{e.id}</span>
                    {i === 0 && <Badge tone="ok">{t("history.current")}</Badge>}
                    <span className="spacer" />
                    <span className="small diffstat">
                      <span className="tone-text-ok">+{e.added}</span> <span className="tone-text-error">−{e.removed}</span>
                    </span>
                  </div>
                  <div className="history-msg">{e.message || e.ops?.join(", ") || "—"}</div>
                  <div className="muted small">
                    {fmtTime(e.time)}
                    {e.user ? " · " + e.user : ""}
                    {e.reloaded ? " · " + t("history.reloaded") : ""}
                  </div>
                </button>
              </li>
            ))}
          </ol>
          <div className="history-detail">{selected !== undefined && <Detail id={selected} isCurrent={entries[0]?.id === selected} />}</div>
        </div>
      )}
    </div>
  );
}

function Detail({ id, isCurrent }: { id: number; isCurrent: boolean }) {
  const { t, fmtTime, errorText } = useT();
  const { config, refreshAll } = useApp();
  const toast = useToast();
  const [data, setData] = useState<{ entry: HistoryEntry; content: string; diff: DiffResult; diffToCurrent?: DiffResult; current?: string } | null>(null);
  const [tab, setTab] = useState<"change" | "current" | "content">("change");
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<unknown>(null);
  useEffect(() => {
    setData(null);
    setErr(null);
    api.historyEntry(id).then(setData).catch(setErr);
  }, [id, config?.version]);
  if (err != null) return <div className="notice notice-error">{errorText(err)}</div>;
  if (!data) return <Spinner />;
  const e = data.entry;
  const same = (data.diffToCurrent?.added ?? 0) + (data.diffToCurrent?.removed ?? 0) === 0;
  const restore = async () => {
    setBusy(true);
    try {
      const r = await api.rollback(id, data.current ?? config?.version ?? "");
      if (r.reloadError) toast({ tone: "warn", title: t("toast.savedReloadFailed"), body: r.reloadError, details: r.reloadOutput });
      else toast({ tone: "ok", title: t("history.restored", { id }) });
      await refreshAll();
      setConfirm(false);
    } catch (e2) {
      toast({ tone: "error", title: t("history.restoreFailed"), body: errorText(e2) });
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="card">
      <header className="detail-head">
        <div>
          <h2 className="card-title">
            {t("history.version", { id: e.id })} <Badge tone={kindTone(e.kind)}>{t(("history.kind." + e.kind) as Key)}</Badge>
          </h2>
          <div className="muted small">
            {fmtTime(e.time)}
            {e.user ? " · " + e.user : ""} · <span className="mono">{e.hash}</span>
          </div>
          {e.message && <p>{e.message}</p>}
          {e.ops && e.ops.length > 0 && (
            <ul className="plain mono small">
              {e.ops.map((o, i) => (
                <li key={i}>{o}</li>
              ))}
            </ul>
          )}
        </div>
        {!isCurrent && !same && (
          <button type="button" className="btn btn-primary" onClick={() => setConfirm(true)}>
            <Icon name="restore" /> {t("history.restore")}
          </button>
        )}
      </header>
      <div className="tabs" role="tablist">
        {(["change", "current", "content"] as const).map((k) => (
          <button key={k} type="button" role="tab" aria-selected={tab === k} className={tab === k ? "active" : ""} onClick={() => setTab(k)}>
            {t(("history.tab." + k) as Key)}
          </button>
        ))}
      </div>
      <div role="tabpanel">
        {tab === "change" && <DiffView diff={data.diff} maxHeight={560} />}
        {tab === "current" && (same ? <p className="muted">{t("history.sameAsCurrent")}</p> : data.diffToCurrent && <DiffView diff={data.diffToCurrent} maxHeight={560} />)}
        {tab === "content" && <pre className="code-view">{data.content}</pre>}
      </div>
      {confirm && (
        <Dialog
          title={t("history.restoreTitle", { id })}
          onClose={() => setConfirm(false)}
          wide
          footer={
            <>
              <button type="button" className="btn" onClick={() => setConfirm(false)}>
                {t("common.cancel")}
              </button>
              <button type="button" className="btn btn-primary" onClick={restore} disabled={busy} data-autofocus>
                {busy ? <Spinner /> : <Icon name="restore" />} {t("history.restore")}
              </button>
            </>
          }
        >
          <p>{t("history.restoreBody")}</p>
          {data.diffToCurrent && <DiffView diff={invert(data.diffToCurrent)} maxHeight={380} />}
        </Dialog>
      )}
      <p className="muted small">
        <Link to="/config">{t("history.openEditor")}</Link>
      </p>
    </section>
  );
}

/** diffToCurrent goes version → current; for a restore we show current → version. */
function invert(d: DiffResult): DiffResult {
  return {
    added: d.removed,
    removed: d.added,
    hunks: d.hunks.map((h) => ({
      oldStart: h.newStart,
      newStart: h.oldStart,
      lines: h.lines.map((l) => ({ op: l.op === "+" ? "-" : l.op === "-" ? "+" : "=", text: l.text, old: l.new, new: l.old })),
    })),
  };
}
