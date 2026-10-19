import {
  Box,
  Camera,
  Download,
  Eye,
  EyeOff,
  Grid3x3,
  LoaderCircle,
  Palette,
  RotateCw,
  Triangle,
  X,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useFile } from "../api/library";
import { formatBytes, formatNumber } from "../lib/format";
import { useStoredState } from "../lib/hooks";
import { type Geometry, fetchGeometry, partStats } from "./geometry";
import { ModelScene, type ViewPreset } from "./scene";

const views: [ViewPreset, string][] = [
  ["iso", "Iso"],
  ["front", "Front"],
  ["back", "Back"],
  ["left", "Left"],
  ["right", "Right"],
  ["top", "Top"],
  ["bottom", "Bottom"],
];

const backgrounds: [string, string][] = [
  ["#16161b", "Dark"],
  ["#2b2d33", "Graphite"],
  ["#8a8f99", "Studio grey"],
  ["#f2f2f5", "Light"],
];

/** Full-screen 3D viewer for one file. Loaded lazily, as it pulls in three.js. */
export default function ModelViewer({ fileId, onClose }: { fileId: number; onClose: () => void }) {
  const container = useRef<HTMLDivElement>(null);
  const sceneRef = useRef<ModelScene | null>(null);
  const { data: file } = useFile(fileId);

  const [geometry, setGeometry] = useState<Geometry | null>(null);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const [plate, setPlate] = useState(0); // 0 = all plates
  const [hidden, setHidden] = useState<Set<number>>(new Set());
  const [view, setView] = useState<ViewPreset>("iso");
  const [ortho, setOrtho] = useStoredState("mm.viewer.ortho", false);
  const [grid, setGrid] = useStoredState("mm.viewer.grid", true);
  const [wire, setWire] = useState(false);
  const [spin, setSpin] = useState(false);
  const [background, setBackground] = useStoredState("mm.viewer.bg", backgrounds[0][0]);
  const [color, setColor] = useStoredState<string | null>(`mm.color.${fileId}`, null);

  // Scene lifetime.
  useEffect(() => {
    if (!container.current) return;
    const scene = new ModelScene(container.current);
    sceneRef.current = scene;
    return () => {
      scene.dispose();
      sceneRef.current = null;
    };
  }, []);

  // Download the model.
  useEffect(() => {
    const ctrl = new AbortController();
    setGeometry(null);
    setError(null);
    setProgress(0);
    fetchGeometry(fileId, setProgress, ctrl.signal)
      .then((g) => {
        setGeometry(g);
        // Multi-plate projects open on their first plate.
        setPlate(g.header.plates.length > 1 ? (g.header.plates[0]?.index ?? 0) : 0);
        setHidden(new Set());
      })
      .catch((e: Error) => {
        if (!ctrl.signal.aborted) setError(e.message);
      });
    return () => ctrl.abort();
  }, [fileId]);

  useEffect(() => {
    if (geometry) sceneRef.current?.setModel(geometry);
  }, [geometry]);

  // Keep the scene in sync with the controls.
  useEffect(() => {
    if (!geometry) return;
    const parts = geometry.header.parts;
    sceneRef.current?.setVisibleParts((i) => !hidden.has(i) && (plate === 0 || parts[i].plate === plate));
  }, [geometry, plate, hidden]);
  useEffect(() => sceneRef.current?.setOrthographic(ortho), [ortho, geometry]);
  useEffect(() => sceneRef.current?.setGrid(grid), [grid, geometry]);
  useEffect(() => sceneRef.current?.setWireframe(wire), [wire, geometry]);
  useEffect(() => sceneRef.current?.setAutoRotate(spin), [spin]);
  useEffect(() => sceneRef.current?.setBackground(background), [background, geometry]);
  useEffect(() => sceneRef.current?.setOverrideColor(color), [color, geometry]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const stats = useMemo(() => (geometry ? partStats(geometry) : []), [geometry]);
  const visibleIdx = useMemo(() => {
    if (!geometry) return [];
    return geometry.header.parts
      .map((p, i) => ({ p, i }))
      .filter(({ p, i }) => !hidden.has(i) && (plate === 0 || p.plate === plate))
      .map(({ i }) => i);
  }, [geometry, plate, hidden]);

  async function screenshot() {
    const blob = await sceneRef.current?.screenshot();
    if (!blob) return;
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${(file?.name ?? "model").replace(/\.[^.]+$/, "")}.png`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  }

  const plates = geometry?.header.plates ?? [];
  const parts = geometry?.header.parts ?? [];
  const partsOnPlate = parts.map((p, i) => ({ p, i })).filter(({ p }) => plate === 0 || p.plate === plate);
  const hasFileColors = parts.some((p) => p.color);

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-bg lg:flex-row" role="dialog" aria-label="3D viewer">
      <div className="relative min-h-0 flex-1">
        <div ref={container} className="absolute inset-0" />

        {/* Toolbar */}
        <div className="absolute top-3 left-3 flex flex-wrap items-center gap-1.5">
          <div className="flex overflow-hidden rounded-lg border border-border bg-surface/90 text-xs backdrop-blur">
            {views.map(([v, label]) => (
              <button
                key={v}
                onClick={() => {
                  setView(v);
                  sceneRef.current?.setView(v);
                }}
                className={`px-2.5 py-1.5 ${view === v ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"}`}
              >
                {label}
              </button>
            ))}
          </div>
          <div className="flex overflow-hidden rounded-lg border border-border bg-surface/90 backdrop-blur">
            <ToolButton active={ortho} onClick={() => setOrtho(!ortho)} label="Orthographic view">
              <Box className="size-4" />
            </ToolButton>
            <ToolButton active={grid} onClick={() => setGrid(!grid)} label="Build plate grid">
              <Grid3x3 className="size-4" />
            </ToolButton>
            <ToolButton active={wire} onClick={() => setWire(!wire)} label="Wireframe">
              <Triangle className="size-4" />
            </ToolButton>
            <ToolButton active={spin} onClick={() => setSpin(!spin)} label="Spin">
              <RotateCw className="size-4" />
            </ToolButton>
            <label
              className="relative flex cursor-pointer items-center px-2.5 text-muted hover:text-fg"
              title={hasFileColors ? "Override the file's colours" : "Model colour"}
            >
              <Palette className="size-4" style={color ? { color } : undefined} />
              <input
                type="color"
                value={color ?? "#a3b1d6"}
                onChange={(e) => setColor(e.target.value)}
                className="absolute inset-0 cursor-pointer opacity-0"
                aria-label="Model colour"
              />
            </label>
            <select
              value={background}
              onChange={(e) => setBackground(e.target.value)}
              className="border-l border-border bg-transparent px-2 text-xs text-muted outline-none"
              aria-label="Background"
            >
              {backgrounds.map(([v, label]) => (
                <option key={v} value={v}>
                  {label}
                </option>
              ))}
            </select>
            <ToolButton onClick={screenshot} label="Save PNG screenshot">
              <Camera className="size-4" />
            </ToolButton>
          </div>
          {color && (
            <button onClick={() => setColor(null)} className="text-xs text-muted underline hover:text-fg">
              {hasFileColors ? "Use file colours" : "Reset colour"}
            </button>
          )}
        </div>

        <button
          onClick={onClose}
          className="absolute top-3 right-3 rounded-lg border border-border bg-surface/90 p-2 text-muted backdrop-blur hover:text-fg"
          aria-label="Close viewer"
        >
          <X className="size-5" />
        </button>

        {plates.length > 1 && (
          <div className="absolute bottom-3 left-3 flex flex-wrap gap-1.5">
            {[{ index: 0, name: "All plates" }, ...plates].map((p) => (
              <button
                key={p.index}
                onClick={() => setPlate(p.index)}
                className={`rounded-full border px-3 py-1 text-xs backdrop-blur ${
                  plate === p.index
                    ? "border-accent bg-accent text-accent-fg"
                    : "border-border bg-surface/90 text-muted hover:text-fg"
                }`}
              >
                {p.index === 0 ? p.name : p.name || `Plate ${p.index}`}
              </button>
            ))}
          </div>
        )}

        {!geometry && (
          <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-3">
            {error ? (
              <p className="pointer-events-auto max-w-sm rounded-lg bg-danger/10 px-4 py-3 text-center text-sm text-danger">
                {error}
              </p>
            ) : (
              <>
                <LoaderCircle className="size-8 animate-spin text-muted" />
                <div className="h-1.5 w-48 overflow-hidden rounded-full bg-surface-2">
                  <div className="h-full bg-accent transition-all" style={{ width: `${progress * 100}%` }} />
                </div>
                <p className="text-xs text-muted">Loading model…</p>
              </>
            )}
          </div>
        )}
        <p className="pointer-events-none absolute right-3 bottom-3 hidden text-[11px] text-muted sm:block">
          Drag to rotate · right-drag to pan · scroll to zoom
        </p>
      </div>

      <aside className="max-h-[42vh] shrink-0 overflow-y-auto border-t border-border bg-surface p-4 lg:max-h-none lg:w-80 lg:border-t-0 lg:border-l">
        <h2 className="truncate font-semibold" title={file?.name}>
          {file?.name ?? "…"}
        </h2>
        <p className="mb-4 truncate text-xs text-muted" title={file?.rel_path}>
          {file ? `${file.source_name} / ${file.rel_path}` : ""}
        </p>

        {geometry && (
          <ModelInfo
            stats={visibleIdx.map((i) => stats[i])}
            triangles={visibleIdx.reduce((n, i) => n + parts[i].count, 0)}
            watertight={geometry.header.watertight}
            fileSize={file?.size}
          />
        )}

        {partsOnPlate.length > 1 && (
          <div className="mt-5">
            <h3 className="mb-2 text-sm font-medium">Parts</h3>
            <ul className="space-y-1">
              {partsOnPlate.map(({ p, i }) => (
                <li key={i} className="flex items-center gap-2 text-sm">
                  <span
                    className="size-3 shrink-0 rounded-sm border border-border"
                    style={{ background: color ?? p.color ?? "#a3b1d6" }}
                  />
                  <span className={`min-w-0 flex-1 truncate ${hidden.has(i) ? "text-muted line-through" : ""}`}>
                    {p.name || `Part ${i + 1}`}
                  </span>
                  <button
                    onClick={() => {
                      const next = new Set(hidden);
                      if (next.has(i)) next.delete(i);
                      else next.add(i);
                      setHidden(next);
                    }}
                    className="rounded p-1 text-muted hover:text-fg"
                    aria-label={hidden.has(i) ? `Show ${p.name}` : `Hide ${p.name}`}
                  >
                    {hidden.has(i) ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}

        <a
          href={`/api/files/${fileId}/download`}
          download={file?.name}
          className="mt-5 flex items-center justify-center gap-2 rounded-lg border border-border px-3.5 py-2 text-sm hover:bg-surface-2"
        >
          <Download className="size-4" aria-hidden /> Download {file ? `(${formatBytes(file.size)})` : ""}
        </a>
      </aside>
    </div>
  );
}

function ToolButton({
  active,
  onClick,
  label,
  children,
}: {
  active?: boolean;
  onClick: () => void;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      title={label}
      aria-label={label}
      aria-pressed={active}
      className={`px-2.5 py-1.5 ${active ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"}`}
    >
      {children}
    </button>
  );
}

// g/cm³
const MATERIALS: [string, number][] = [
  ["PLA", 1.24],
  ["PETG", 1.27],
  ["ABS", 1.04],
  ["ASA", 1.07],
  ["TPU", 1.21],
  ["Nylon", 1.14],
  ["Resin", 1.12],
];
const WALL_MM = 1.2; // ~3 perimeters at 0.4 mm, also used for top/bottom shells

/** Dimensions, volume and an estimated print weight for the visible parts. */
function ModelInfo({
  stats,
  triangles,
  watertight,
  fileSize,
}: {
  stats: { volume: number; area: number; min: number[]; max: number[] }[];
  triangles: number;
  watertight: boolean | null;
  fileSize?: number;
}) {
  const [material, setMaterial] = useStoredState("mm.viewer.material", "PLA");
  const [infill, setInfill] = useStoredState("mm.viewer.infill", 15);

  const min = [0, 1, 2].map((d) => Math.min(...stats.map((s) => s.min[d])));
  const max = [0, 1, 2].map((d) => Math.max(...stats.map((s) => s.max[d])));
  const size = max.map((v, d) => v - min[d]);
  const volume = Math.abs(stats.reduce((v, s) => v + s.volume, 0));
  const area = stats.reduce((a, s) => a + s.area, 0);

  // Shell (walls, top, bottom) printed solid, the rest at the infill percentage.
  const shell = Math.min(volume, area * WALL_MM);
  const printed = shell + (volume - shell) * (infill / 100);
  const density = MATERIALS.find(([m]) => m === material)?.[1] ?? 1.24;
  const grams = (printed / 1000) * density;
  const fmt = (v: number) => (v >= 100 ? Math.round(v) : v.toFixed(1));

  if (stats.length === 0) return <p className="text-sm text-muted">No visible parts.</p>;

  return (
    <div className="space-y-4 text-sm">
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
        <dt className="text-muted">Size</dt>
        <dd>
          {size.map(fmt).join(" × ")} mm
          <span className="block text-xs text-muted">W × D × H</span>
        </dd>
        <dt className="text-muted">Volume</dt>
        <dd>{(volume / 1000).toFixed(1)} cm³</dd>
        <dt className="text-muted">Surface</dt>
        <dd>{(area / 100).toFixed(0)} cm²</dd>
        <dt className="text-muted">Triangles</dt>
        <dd>{formatNumber(triangles)}</dd>
        {fileSize !== undefined && (
          <>
            <dt className="text-muted">File</dt>
            <dd>{formatBytes(fileSize)}</dd>
          </>
        )}
        <dt className="text-muted">Mesh</dt>
        <dd>
          {watertight === null ? (
            <span className="text-muted">Too large to check</span>
          ) : watertight ? (
            <span className="text-success">Closed (watertight)</span>
          ) : (
            <span className="text-warning">Open: volume and weight are rough</span>
          )}
        </dd>
      </dl>

      <div className="rounded-lg border border-border p-3">
        <p className="mb-2 font-medium">
          ≈ {grams >= 100 ? Math.round(grams) : grams.toFixed(1)} g
          <span className="ml-1 text-xs font-normal text-muted">estimated print weight</span>
        </p>
        <div className="flex items-center gap-2">
          <select
            value={material}
            onChange={(e) => setMaterial(e.target.value)}
            className="rounded-md border border-border bg-surface px-2 py-1 text-xs"
            aria-label="Material"
          >
            {MATERIALS.map(([m]) => (
              <option key={m}>{m}</option>
            ))}
          </select>
          <input
            type="range"
            min={0}
            max={100}
            step={5}
            value={infill}
            onChange={(e) => setInfill(Number(e.target.value))}
            className="min-w-0 flex-1 accent-accent"
            aria-label="Infill"
          />
          <span className="w-16 text-right text-xs tabular-nums">{infill}% infill</span>
        </div>
        <p className="mt-1.5 text-[11px] text-muted">Assumes {WALL_MM} mm walls and top/bottom shells.</p>
      </div>
    </div>
  );
}
