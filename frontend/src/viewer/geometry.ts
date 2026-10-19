export type GeometryPart = {
  name: string;
  plate: number;
  color?: string;
  start: number;
  count: number;
};

export type GeometryHeader = {
  version: number;
  triangles: number;
  parts: GeometryPart[];
  plates: { index: number; name?: string }[];
  min: [number, number, number];
  max: [number, number, number];
  volume_mm3: number;
  area_mm2: number;
  watertight: boolean | null;
};

export type Geometry = {
  header: GeometryHeader;
  /** 9 floats per triangle, Z-up, millimetres. */
  positions: Float32Array;
};

/**
 * Downloads a model's geometry (see backend handleGeometry for the format):
 * "MMG1" | uint32 header length | header JSON | float32 triangles.
 * onProgress receives a 0..1 fraction once the header has arrived.
 */
export async function fetchGeometry(
  id: number,
  onProgress: (fraction: number) => void,
  signal: AbortSignal,
): Promise<Geometry> {
  const res = await fetch(`/api/files/${id}/geometry`, { credentials: "same-origin", signal });
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new Error(body?.error ?? `Could not load the model (${res.status})`);
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
      const head = concat(chunks, Math.min(received, 8));
      const headerLen = new DataView(head.buffer).getUint32(4, true);
      if (received >= 8 + headerLen) {
        const header = parseHeader(concat(chunks, 8 + headerLen), headerLen);
        expected = 8 + headerLen + header.triangles * 36;
      }
    }
    if (expected) onProgress(Math.min(1, received / expected));
  }

  const bytes = concat(chunks, received);
  if (String.fromCharCode(...bytes.subarray(0, 4)) !== "MMG1") throw new Error("Not a geometry response");
  const headerLen = new DataView(bytes.buffer).getUint32(4, true);
  const header = parseHeader(bytes, headerLen);
  const offset = 8 + headerLen;
  if (bytes.byteLength < offset + header.triangles * 36) throw new Error("Model download was cut short");
  const positions = new Float32Array(bytes.buffer, offset, header.triangles * 9);
  return { header, positions };
}

function parseHeader(bytes: Uint8Array, headerLen: number): GeometryHeader {
  return JSON.parse(new TextDecoder().decode(bytes.subarray(8, 8 + headerLen)));
}

/** First `length` bytes of the chunks, as one buffer that starts at offset 0. */
function concat(chunks: Uint8Array[], length: number): Uint8Array {
  const out = new Uint8Array(length);
  let at = 0;
  for (const c of chunks) {
    if (at >= length) break;
    const take = Math.min(c.length, length - at);
    out.set(c.subarray(0, take), at);
    at += take;
  }
  return out;
}

export type PartStats = { volume: number; area: number; min: number[]; max: number[] };

/** Signed volume (closed meshes), surface area and bounds of each part. */
export function partStats(g: Geometry): PartStats[] {
  const p = g.positions;
  return g.header.parts.map((part) => {
    let volume = 0;
    let area = 0;
    const min = [Infinity, Infinity, Infinity];
    const max = [-Infinity, -Infinity, -Infinity];
    const end = (part.start + part.count) * 9;
    for (let i = part.start * 9; i < end; i += 9) {
      const ax = p[i], ay = p[i + 1], az = p[i + 2];
      const bx = p[i + 3], by = p[i + 4], bz = p[i + 5];
      const cx = p[i + 6], cy = p[i + 7], cz = p[i + 8];
      volume += (ax * (by * cz - bz * cy) - ay * (bx * cz - bz * cx) + az * (bx * cy - by * cx)) / 6;
      const ux = bx - ax, uy = by - ay, uz = bz - az;
      const vx = cx - ax, vy = cy - ay, vz = cz - az;
      const nx = uy * vz - uz * vy, ny = uz * vx - ux * vz, nz = ux * vy - uy * vx;
      area += Math.sqrt(nx * nx + ny * ny + nz * nz) / 2;
      for (let k = 0; k < 9; k += 3) {
        for (let d = 0; d < 3; d++) {
          const v = p[i + k + d];
          if (v < min[d]) min[d] = v;
          if (v > max[d]) max[d] = v;
        }
      }
    }
    return { volume, area, min, max };
  });
}
