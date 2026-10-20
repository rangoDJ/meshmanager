import { Box, Camera, ChevronDown, ChevronUp, Download, Footprints, Grid3x3, Pause, Play } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { FileItem } from "../api/library";
import { CopyLocation } from "../components/CopyLocation";
import { formatBytes, formatNumber } from "../lib/format";
import { useStoredState } from "../lib/hooks";
import {
  backgrounds,
  type FileNavigation,
  formatDuration,
  InfoPanel,
  LoadingContent,
  ToolButton,
  useViewerKeys,
  ViewerCorner,
  views,
} from "./chrome";
import { GcodeScene } from "./gcodeScene";
import type { ViewPreset } from "./stage";
import { FEATURE_COLORS, fetchToolpath, type Toolpath } from "./toolpath";

/** Sliced G-code, layer by layer: everything printed so far, current layer on top. */
export default function GcodeViewer({
  file,
  nav,
  onClose,
}: {
  file: FileItem;
  nav?: FileNavigation;
  onClose: () => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const sceneRef = useRef<GcodeScene | null>(null);
  const [tp, setTp] = useState<Toolpath | null>(null);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);

  // Sliced plates of a .gcode.3mf. Plate 0 asks for the first one.
  const [plates, setPlates] = useState<number[]>([]);
  const [plate, setPlate] = useState(0);
  const [layer, setLayer] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [view, setView] = useState<ViewPreset>("iso");
  const [ortho, setOrtho] = useStoredState("mm.viewer.ortho", false);
  const [grid, setGrid] = useStoredState("mm.viewer.grid", true);
  const [travels, setTravels] = useStoredState("mm.gcode.travels", false);
  const [background, setBackground] = useStoredState("mm.viewer.bg", backgrounds[0][0]);

  useViewerKeys(onClose, nav);

  useEffect(() => {
    if (!container.current) return;
    const scene = new GcodeScene(container.current);
    sceneRef.current = scene;
    return () => {
      scene.dispose();
      sceneRef.current = null;
    };
  }, []);

  useEffect(() => {
    const ctrl = new AbortController();
    setTp(null);
    setError(null);
    setProgress(0);
    setPlaying(false);
    fetchToolpath(file.id, plate, setProgress, ctrl.signal)
      .then((t) => {
        setTp(t);
        setPlates(t.header.plates);
        setLayer(t.header.layers.length - 1); // open on the finished print
      })
      .catch((e: Error) => {
        if (!ctrl.signal.aborted) setError(e.message);
      });
    return () => ctrl.abort();
  }, [file.id, plate]);

  useEffect(() => {
    if (tp) sceneRef.current?.setToolpath(tp);
  }, [tp]);
  useEffect(() => sceneRef.current?.showLayer(layer), [layer, tp]);
  useEffect(() => sceneRef.current?.setTravels(travels), [travels, tp]);
  useEffect(() => sceneRef.current?.setOrthographic(ortho), [ortho, tp]);
  useEffect(() => sceneRef.current?.setGrid(grid), [grid, tp]);
  useEffect(() => sceneRef.current?.setBackground(background), [background, tp]);

  const count = tp?.header.layers.length ?? 0;

  // Play: build the print up layer by layer, ~12 layers a second, then stop on top.
  useEffect(() => {
    if (!playing || !count) return;
    const t = setInterval(() => {
      setLayer((l) => {
        if (l >= count - 1) {
          setPlaying(false);
          return l;
        }
        return l + 1;
      });
    }, 80);
    return () => clearInterval(t);
  }, [playing, count]);

  // ↑/↓ step a layer, Page Up/Down ten, Home/End to the bottom and top.
  useEffect(() => {
    if (!count) return;
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === "INPUT" || t.tagName === "SELECT" || t.tagName === "TEXTAREA")) return;
      const step = { ArrowUp: 1, ArrowDown: -1, PageUp: 10, PageDown: -10 }[e.key];
      if (step) {
        e.preventDefault();
        setPlaying(false);
        setLayer((l) => Math.max(0, Math.min(count - 1, l + step)));
      } else if (e.key === "Home" || e.key === "End") {
        e.preventDefault();
        setLayer(e.key === "Home" ? 0 : count - 1);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [count]);

  const current = tp?.header.layers[layer];
  const featureCounts = useMemo(() => {
    if (!tp) return [];
    const counts = new Array(tp.header.features.length).fill(0);
    for (const f of tp.feature) counts[f]++;
    return tp.header.features.map((name, i) => ({ name, count: counts[i] })).filter((f) => f.count > 0);
  }, [tp]);

  async function screenshot() {
    const blob = await sceneRef.current?.screenshot();
    if (!blob) return;
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    const base = file.name.replace(/\.gcode\.3mf$|\.[^.]+$/i, "");
    a.download = `${base}${plates.length > 1 ? `-plate-${tp?.header.plate}` : ""}-layer-${layer + 1}.png`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  }

  const stepLayer = (d: number) => {
    setPlaying(false);
    setLayer((l) => Math.max(0, Math.min(count - 1, l + d)));
  };

  const layerLabel = current && (
    <span className="tabular-nums">
      Layer {layer + 1} <span className="text-muted">of {count}</span>
      <span className="text-muted"> · </span>
      {current.z.toFixed(2)} mm
    </span>
  );

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-bg lg:flex-row" role="dialog" aria-label="G-code layer preview">
      <div className="relative min-h-0 flex-1">
        <div ref={container} className="absolute inset-0" />

        <div className="pointer-events-none absolute top-3 right-3 left-3 flex flex-wrap items-center gap-1.5 pr-40 *:pointer-events-auto">
          <div className="flex max-w-full overflow-x-auto rounded-lg border border-border bg-surface/90 text-xs backdrop-blur [scrollbar-width:none]">
            {views.map(([v, label]) => (
              <button
                key={v}
                onClick={() => {
                  setView(v);
                  sceneRef.current?.setView(v);
                }}
                className={`shrink-0 px-2.5 py-1.5 transition-colors ${view === v ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"}`}
              >
                {label}
              </button>
            ))}
          </div>
          {plates.length > 1 && (
            <div
              role="group"
              aria-label="Plate"
              className="flex max-w-full overflow-x-auto rounded-lg border border-border bg-surface/90 text-xs backdrop-blur [scrollbar-width:none]"
            >
              {plates.map((n) => {
                const active = n === (plate || plates[0]);
                return (
                  <button
                    key={n}
                    onClick={() => setPlate(n)}
                    aria-pressed={active}
                    className={`shrink-0 px-2.5 py-1.5 transition-colors ${active ? "bg-accent text-accent-fg" : "text-muted hover:text-fg"}`}
                  >
                    Plate {n}
                  </button>
                );
              })}
            </div>
          )}
          <div className="flex shrink-0 overflow-hidden rounded-lg border border-border bg-surface/90 backdrop-blur">
            <ToolButton active={travels} onClick={() => setTravels(!travels)} label="Travel moves on this layer">
              <Footprints className="size-4" />
            </ToolButton>
            <ToolButton active={ortho} onClick={() => setOrtho(!ortho)} label="Orthographic view">
              <Box className="size-4" />
            </ToolButton>
            <ToolButton active={grid} onClick={() => setGrid(!grid)} label="Build plate grid">
              <Grid3x3 className="size-4" />
            </ToolButton>
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
            <ToolButton onClick={screenshot} label="Save PNG of this view">
              <Camera className="size-4" />
            </ToolButton>
          </div>
        </div>

        <ViewerCorner nav={nav} onClose={onClose} />

        {/* Layer control: a vertical rail on wide screens, a bar along the bottom on phones. */}
        {tp && current && (
          <>
            <div className="absolute top-16 right-3 bottom-16 hidden w-14 flex-col items-center gap-2 rounded-xl border border-border bg-surface/90 py-2 backdrop-blur lg:flex">
              <button
                onClick={() => stepLayer(1)}
                className="rounded-md p-1.5 text-muted hover:bg-surface-2 hover:text-fg"
                aria-label="Next layer up"
                title="Layer up (↑)"
              >
                <ChevronUp className="size-4" />
              </button>
              <input
                type="range"
                min={0}
                max={count - 1}
                value={layer}
                onChange={(e) => {
                  setPlaying(false);
                  setLayer(Number(e.target.value));
                }}
                aria-label="Layer"
                aria-valuetext={`Layer ${layer + 1} of ${count}, ${current.z.toFixed(2)} millimetres`}
                className="layer-rail min-h-0 flex-1 accent-accent"
              />
              <button
                onClick={() => stepLayer(-1)}
                className="rounded-md p-1.5 text-muted hover:bg-surface-2 hover:text-fg"
                aria-label="Next layer down"
                title="Layer down (↓)"
              >
                <ChevronDown className="size-4" />
              </button>
              <button
                onClick={() => {
                  if (!playing && layer >= count - 1) setLayer(0);
                  setPlaying(!playing);
                }}
                className="rounded-md p-1.5 text-muted hover:bg-surface-2 hover:text-fg"
                aria-label={playing ? "Pause" : "Play the print layer by layer"}
                title={playing ? "Pause" : "Play layer by layer"}
              >
                {playing ? <Pause className="size-4" /> : <Play className="size-4" />}
              </button>
            </div>
            <div className="absolute right-[4.75rem] bottom-3 hidden rounded-lg border border-border bg-surface/90 px-3 py-1.5 text-sm backdrop-blur lg:block">
              {layerLabel}
            </div>

            <div className="absolute inset-x-3 bottom-3 rounded-xl border border-border bg-surface/90 px-1.5 pt-1.5 pb-1 backdrop-blur lg:hidden">
              <p className="px-1.5 text-center text-xs" aria-hidden>
                {layerLabel}
              </p>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => {
                    if (!playing && layer >= count - 1) setLayer(0);
                    setPlaying(!playing);
                  }}
                  className="grid size-10 shrink-0 place-items-center rounded-lg text-muted hover:text-fg"
                  aria-label={playing ? "Pause" : "Play the print layer by layer"}
                >
                  {playing ? <Pause className="size-4" /> : <Play className="size-4" />}
                </button>
                <button
                  onClick={() => stepLayer(-1)}
                  className="grid size-10 shrink-0 place-items-center rounded-lg text-muted hover:text-fg"
                  aria-label="Next layer down"
                >
                  <ChevronDown className="size-5" />
                </button>
                <input
                  type="range"
                  min={0}
                  max={count - 1}
                  value={layer}
                  onChange={(e) => {
                    setPlaying(false);
                    setLayer(Number(e.target.value));
                  }}
                  aria-label="Layer"
                  aria-valuetext={`Layer ${layer + 1} of ${count}, ${current.z.toFixed(2)} millimetres`}
                  className="min-w-0 flex-1 accent-accent"
                />
                <button
                  onClick={() => stepLayer(1)}
                  className="grid size-10 shrink-0 place-items-center rounded-lg text-muted hover:text-fg"
                  aria-label="Next layer up"
                >
                  <ChevronUp className="size-5" />
                </button>
              </div>
            </div>
          </>
        )}

        {!tp &&
          (error ? (
            <div className="absolute inset-0 flex items-center justify-center p-6">
              <div className="max-w-sm rounded-lg bg-danger/10 px-4 py-3 text-center text-sm text-danger">
                <p>{error}</p>
                <p className="mt-1 text-xs opacity-80">You can still download the file and open it in your slicer.</p>
              </div>
            </div>
          ) : (
            <LoadingContent progress={progress || undefined} label="Reading toolpaths…" />
          ))}
      </div>

      <InfoPanel>
        <h2 className="truncate font-semibold" title={file.name}>
          {file.name}
        </h2>
        <p className="mb-4 text-xs text-muted">{tp?.header.slicer ? `Sliced with ${tp.header.slicer}` : "Sliced G-code"}</p>

        {tp && (
          <div className="space-y-5 text-sm">
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
              {tp.header.print_seconds > 0 && (
                <>
                  <dt className="text-muted">Print time</dt>
                  <dd>{formatDuration(tp.header.print_seconds)}</dd>
                </>
              )}
              {(tp.header.filament_grams > 0 || tp.header.filament_mm > 0) && (
                <>
                  <dt className="text-muted">Filament</dt>
                  <dd className="tabular-nums">
                    {tp.header.filament_grams > 0 && `${tp.header.filament_grams.toFixed(1)} g`}
                    {tp.header.filament_grams > 0 && tp.header.filament_mm > 0 && " · "}
                    {tp.header.filament_mm > 0 && `${(tp.header.filament_mm / 1000).toFixed(2)} m`}
                  </dd>
                </>
              )}
              <dt className="text-muted">Layers</dt>
              <dd className="tabular-nums">{formatNumber(count)}</dd>
              <dt className="text-muted">Size</dt>
              <dd className="tabular-nums">
                {[0, 1, 2].map((d) => (tp.header.max[d] - tp.header.min[d]).toFixed(1)).join(" × ")} mm
              </dd>
            </dl>

            {current && (
              <section aria-label="Current layer">
                <h3 className="mb-1.5 font-medium">{layerLabel}</h3>
                <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
                  <dt className="text-muted">Layer height</dt>
                  <dd className="tabular-nums">{current.height.toFixed(2)} mm</dd>
                  <dt className="text-muted">Moves</dt>
                  <dd className="tabular-nums">
                    {formatNumber(current.extrusion_count)} printing · {formatNumber(current.travel_count)} travel
                  </dd>
                </dl>
              </section>
            )}

            {featureCounts.length > 0 && (
              <section aria-label="Line types">
                <h3 className="mb-1.5 font-medium">Line types</h3>
                <ul className="space-y-1">
                  {featureCounts.map((f) => (
                    <li key={f.name} className="flex items-center gap-2">
                      <span
                        className="h-1 w-4 shrink-0 rounded-full"
                        style={{ background: FEATURE_COLORS[f.name] ?? FEATURE_COLORS.Other }}
                        aria-hidden
                      />
                      <span className="flex-1">{f.name}</span>
                    </li>
                  ))}
                  {travels && (
                    <li className="flex items-center gap-2">
                      <span className="h-1 w-4 shrink-0 rounded-full bg-[#7c8cff] opacity-60" aria-hidden />
                      <span className="flex-1">Travel (this layer)</span>
                    </li>
                  )}
                </ul>
              </section>
            )}
          </div>
        )}

        <div className="mt-5 space-y-3 text-sm">
          <CopyLocation file={file} />
          <a
            href={`/api/files/${file.id}/download`}
            download={file.name}
            className="flex items-center justify-center gap-2 rounded-lg border border-border px-3.5 py-2 hover:bg-surface-2"
          >
            <Download className="size-4" aria-hidden /> Download ({formatBytes(file.size)})
          </a>
        </div>
      </InfoPanel>
    </div>
  );
}
