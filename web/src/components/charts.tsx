import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";
import type { Point } from "../stats";

// ---------------------------------------------------------------------------
// Line chart with crosshair and tooltip

export interface Series {
  name: string;
  color: string; // CSS color, e.g. "var(--chart-1)"
  points: Point[];
  area?: boolean;
}

function niceMax(v: number): number {
  if (v <= 0) return 1;
  const exp = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 4, 5, 6, 8, 10]) {
    if (m * exp >= v) return m * exp;
  }
  return 10 * exp;
}

function useWidth<T extends HTMLElement>(): [React.RefObject<T>, number] {
  const ref = useRef<T>(null);
  const [w, setW] = useState(600);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const cw = Math.floor(entries[0].contentRect.width);
      if (cw > 0) setW(cw);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, w];
}

export function LineChart({
  series,
  height = 200,
  format,
  timeFormat,
  label,
  emptyText,
}: {
  series: Series[];
  height?: number;
  format: (v: number | null) => string;
  timeFormat: (t: number) => string;
  label: string;
  emptyText: string;
}) {
  const [wrapRef, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const pad = { top: 12, right: 12, bottom: 24, left: 48 };
  const times = series[0]?.points.map((p) => p.t) ?? [];
  const n = times.length;
  const max = useMemo(() => {
    let m = 0;
    for (const s of series) for (const p of s.points) if (p.v !== null && p.v > m) m = p.v;
    return niceMax(m);
  }, [series]);
  const iw = Math.max(10, width - pad.left - pad.right);
  const ih = height - pad.top - pad.bottom;
  const x = (i: number) => pad.left + (n <= 1 ? iw : (i / (n - 1)) * iw);
  const y = (v: number) => pad.top + ih - (v / max) * ih;

  const paths = series.map((s) => {
    let d = "";
    let area = "";
    let open = false;
    let segStart = 0;
    s.points.forEach((p, i) => {
      if (p.v === null) {
        if (open && s.area) area += `L${x(i - 1)},${y(0)}L${x(segStart)},${y(0)}Z`;
        open = false;
        return;
      }
      if (!open) {
        d += `M${x(i)},${y(p.v)}`;
        if (s.area) area += `M${x(i)},${y(p.v)}`;
        segStart = i;
        open = true;
      } else {
        d += `L${x(i)},${y(p.v)}`;
        if (s.area) area += `L${x(i)},${y(p.v)}`;
      }
    });
    if (open && s.area) area += `L${x(n - 1)},${y(0)}L${x(segStart)},${y(0)}Z`;
    return { d, area };
  });

  const ticks = [0, max / 2, max];
  const onMove = (e: PointerEvent<SVGSVGElement>) => {
    if (n === 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const px = ((e.clientX - rect.left) / rect.width) * width;
    const i = Math.round(((px - pad.left) / iw) * (n - 1));
    setHover(Math.max(0, Math.min(n - 1, i)));
  };
  const onKey = (e: KeyboardEvent<SVGSVGElement>) => {
    if (n === 0) return;
    if (e.key === "ArrowLeft") setHover((h) => Math.max(0, (h ?? n - 1) - 1));
    else if (e.key === "ArrowRight") setHover((h) => Math.min(n - 1, (h ?? n - 1) + 1));
    else if (e.key === "Escape") setHover(null);
    else return;
    e.preventDefault();
  };
  const hasData = series.some((s) => s.points.some((p) => p.v !== null));
  const tipLeft = hover !== null ? x(hover) : 0;

  return (
    <div className="chart" ref={wrapRef}>
      {series.length > 1 && (
        <div className="chart-legend" aria-hidden="true">
          {series.map((s) => (
            <span key={s.name} className="chart-legend-item">
              <span className="chart-swatch" style={{ background: s.color }} />
              {s.name}
            </span>
          ))}
        </div>
      )}
      <div className="chart-plot">
        <svg
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label={label}
          tabIndex={0}
          onPointerMove={onMove}
          onPointerLeave={() => setHover(null)}
          onKeyDown={onKey}
          onBlur={() => setHover(null)}
        >
          {ticks.map((tv, k) => (
            <g key={k}>
              <line x1={pad.left} x2={pad.left + iw} y1={y(tv)} y2={y(tv)} className={k === 0 ? "chart-baseline" : "chart-grid"} />
              <text x={pad.left - 8} y={y(tv)} className="chart-axis" textAnchor="end" dominantBaseline="middle">
                {format(tv)}
              </text>
            </g>
          ))}
          {n > 1 &&
            [0, Math.floor((n - 1) / 2), n - 1].map((i, k) => (
              <text key={k} x={x(i)} y={height - 6} className="chart-axis" textAnchor={k === 0 ? "start" : k === 2 ? "end" : "middle"}>
                {timeFormat(times[i])}
              </text>
            ))}
          {paths.map((p, k) =>
            series[k].area ? <path key={"a" + k} d={p.area} fill={series[k].color} opacity={0.12} /> : null,
          )}
          {paths.map((p, k) => (
            <path key={"l" + k} d={p.d} fill="none" stroke={series[k].color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
          ))}
          {hover !== null && (
            <g>
              <line x1={x(hover)} x2={x(hover)} y1={pad.top} y2={pad.top + ih} className="chart-crosshair" />
              {series.map((s) => {
                const v = s.points[hover]?.v;
                return v === null || v === undefined ? null : (
                  <circle key={s.name} cx={x(hover)} cy={y(v)} r={4} fill={s.color} stroke="var(--surface)" strokeWidth={2} />
                );
              })}
            </g>
          )}
        </svg>
        {!hasData && <div className="chart-empty muted">{emptyText}</div>}
        {hover !== null && hasData && (
          <div
            className="chart-tip"
            style={{ left: Math.min(Math.max(tipLeft, 70), width - 70) }}
            role="status"
            aria-live="polite"
          >
            <div className="chart-tip-time muted">{timeFormat(times[hover])}</div>
            {series.map((s) => (
              <div key={s.name} className="chart-tip-row">
                <span className="chart-swatch" style={{ background: s.color }} />
                <strong>{format(s.points[hover]?.v ?? null)}</strong>
                <span className="muted">{s.name}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sparkline: recent trend of one value, latest point highlighted

export function Sparkline({ points, label, width = 84, height = 24 }: { points: Point[]; label: string; width?: number; height?: number }) {
  const vals = points.map((p) => p.v);
  const max = Math.max(0, ...vals.map((v) => v ?? 0));
  if (points.length < 2) return <span className="spark-empty muted">–</span>;
  const x = (i: number) => 2 + (i / (points.length - 1)) * (width - 4);
  const y = (v: number) => height - 3 - (max > 0 ? (v / max) * (height - 6) : 0);
  let d = "";
  let open = false;
  vals.forEach((v, i) => {
    if (v === null) {
      open = false;
      return;
    }
    d += `${open ? "L" : "M"}${x(i)},${y(v)}`;
    open = true;
  });
  const last = vals[vals.length - 1];
  return (
    <svg className="spark" width={width} height={height} viewBox={`0 0 ${width} ${height}`} role="img" aria-label={label}>
      <title>{label}</title>
      <path d={d} fill="none" stroke="var(--neutral)" strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" opacity={0.7} />
      {last !== null && <circle cx={x(vals.length - 1)} cy={y(last)} r={2.5} fill="var(--chart-1)" />}
    </svg>
  );
}

// ---------------------------------------------------------------------------
// Segmented bar of HTTP response classes

export interface CodeCounts {
  c2: number;
  c3: number;
  c4: number;
  c5: number;
  other: number;
}

export function CodeBar({ counts, labels, fmt }: { counts: CodeCounts; labels: Record<keyof CodeCounts, string>; fmt: (n: number) => string }) {
  const parts: { key: keyof CodeCounts; tone: string }[] = [
    { key: "c2", tone: "var(--ok)" },
    { key: "c3", tone: "var(--neutral)" },
    { key: "c4", tone: "var(--warn)" },
    { key: "c5", tone: "var(--error)" },
    { key: "other", tone: "var(--border-strong)" },
  ];
  const total = parts.reduce((s, p) => s + counts[p.key], 0);
  if (total === 0) return <span className="muted">–</span>;
  const desc = parts
    .filter((p) => counts[p.key] > 0)
    .map((p) => `${labels[p.key]}: ${fmt(counts[p.key])} (${Math.round((counts[p.key] / total) * 100)}%)`)
    .join(", ");
  return (
    <div className="codebar" role="img" aria-label={desc} title={desc}>
      {parts.map((p) =>
        counts[p.key] > 0 ? <span key={p.key} style={{ flexGrow: counts[p.key], background: p.tone }} /> : null,
      )}
    </div>
  );
}
