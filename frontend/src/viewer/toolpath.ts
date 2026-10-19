export type ToolpathLayer = {
  z: number;
  height: number;
  extrusion_start: number;
  extrusion_count: number;
  travel_start: number;
  travel_count: number;
};

export type ToolpathHeader = {
  version: number;
  layers: ToolpathLayer[];
  features: string[];
  extrusions: number;
  travels: number;
  min: [number, number, number];
  max: [number, number, number];
  slicer: string;
  print_seconds: number;
  filament_grams: number;
  filament_mm: number;
};

export type Toolpath = {
  header: ToolpathHeader;
  /** x1, y1, x2, y2 per extrusion segment (mm); Z comes from the layer. */
  extrusions: Float32Array;
  travels: Float32Array;
  /** Feature index per extrusion segment (into header.features). */
  feature: Uint8Array;
};

// Slicer-style colours per feature, in the order of header.features.
export const FEATURE_COLORS: Record<string, string> = {
  Other: "#9aa0ab",
  "Outer wall": "#f59e0b",
  "Inner wall": "#fcd34d",
  Infill: "#ef4444",
  "Solid infill": "#a855f7",
  "Top/bottom": "#ec4899",
  Bridge: "#3b82f6",
  Support: "#22c55e",
  "Skirt/brim": "#14b8a6",
  Overhang: "#0ea5e9",
};

/**
 * Downloads a G-code file's toolpath (see backend handleToolpath for the format):
 * "MMT1" | uint32 header length | header JSON | float32 extrusions | float32 travels |
 * uint8 features. onProgress receives 0..1 once the header has arrived.
 */
export async function fetchToolpath(
  id: number,
  onProgress: (fraction: number) => void,
  signal: AbortSignal,
): Promise<Toolpath> {
  const res = await fetch(`/api/files/${id}/toolpath`, { credentials: "same-origin", signal });
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new Error(body?.error ?? `Could not load the G-code (${res.status})`);
  }
  if (!res.body) throw new Error("Empty response");

  const reader = res.body.getReader();
  const chunks: Uint8Array[] = [];
  let received = 0;
  let expected = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    received += value.length;
    if (!expected && received >= 8) {
      const all = join(chunks, received);
      const headerLen = new DataView(all.buffer).getUint32(4, true);
      if (received >= 8 + headerLen) {
        const h = parseHeader(all, headerLen);
        expected = 8 + headerLen + (h.extrusions + h.travels) * 16 + Math.ceil(h.extrusions / 4) * 4;
      }
    }
    if (expected) onProgress(Math.min(1, received / expected));
  }

  const bytes = join(chunks, received);
  if (String.fromCharCode(...bytes.subarray(0, 4)) !== "MMT1") throw new Error("Not a toolpath response");
  const headerLen = new DataView(bytes.buffer).getUint32(4, true);
  const header = parseHeader(bytes, headerLen);
  let at = 8 + headerLen;
  const extrusions = new Float32Array(bytes.buffer, at, header.extrusions * 4);
  at += header.extrusions * 16;
  const travels = new Float32Array(bytes.buffer, at, header.travels * 4);
  at += header.travels * 16;
  if (bytes.byteLength < at + header.extrusions) throw new Error("G-code download was cut short");
  const feature = new Uint8Array(bytes.buffer, at, header.extrusions);
  return { header, extrusions, travels, feature };
}

function parseHeader(bytes: Uint8Array, headerLen: number): ToolpathHeader {
  return JSON.parse(new TextDecoder().decode(bytes.subarray(8, 8 + headerLen)));
}

function join(chunks: Uint8Array[], length: number): Uint8Array {
  const out = new Uint8Array(length);
  let at = 0;
  for (const c of chunks) {
    out.set(c.subarray(0, Math.min(c.length, length - at)), at);
    at += c.length;
    if (at >= length) break;
  }
  return out;
}
