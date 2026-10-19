import * as THREE from "three";
import { Stage, type ViewPreset } from "./stage";
import { FEATURE_COLORS, type Toolpath } from "./toolpath";

/**
 * A sliced print on the shared stage. Layers below the current one are drawn dimmed,
 * the current layer at full colour, and (optionally) the current layer's travel moves.
 */
export class GcodeScene {
  private stage: Stage;
  private below: THREE.LineSegments | null = null;
  private current: THREE.LineSegments | null = null;
  private travel: THREE.LineSegments | null = null;
  private tp: Toolpath | null = null;
  private bounds: THREE.Box3 | null = null;
  private showTravels = false;
  private layer = 0;
  // Lower layers fade toward the background, so the current layer stands out on any backdrop.
  // The mix runs in linear light, where 0.86 reads as roughly two-thirds faded.
  private fade = { value: new THREE.Color("#16161b") };

  constructor(container: HTMLElement) {
    this.stage = new Stage(container, () => this.bounds);
  }

  setToolpath(tp: Toolpath) {
    this.clear();
    this.tp = tp;
    const { layers, features, min, max } = tp.header;

    // Expand 2D segments to 3D at each layer's height, with a colour per vertex.
    const n = tp.header.extrusions;
    const pos = new Float32Array(n * 6);
    const col = new Uint8Array(n * 6);
    const palette = features.map((f) => new THREE.Color(FEATURE_COLORS[f] ?? FEATURE_COLORS.Other));
    for (const layer of layers) {
      for (let i = layer.extrusion_start; i < layer.extrusion_start + layer.extrusion_count; i++) {
        const s = i * 4;
        const o = i * 6;
        pos[o] = tp.extrusions[s];
        pos[o + 1] = tp.extrusions[s + 1];
        pos[o + 2] = layer.z;
        pos[o + 3] = tp.extrusions[s + 2];
        pos[o + 4] = tp.extrusions[s + 3];
        pos[o + 5] = layer.z;
        const c = palette[tp.feature[i]] ?? palette[0];
        const r = Math.round(c.r * 255), g = Math.round(c.g * 255), b = Math.round(c.b * 255);
        col[o] = col[o + 3] = r;
        col[o + 1] = col[o + 4] = g;
        col[o + 2] = col[o + 5] = b;
      }
    }
    const position = new THREE.BufferAttribute(pos, 3);
    const color = new THREE.BufferAttribute(col, 3, true);

    // Two views of the same buffers, each with its own draw range.
    const belowGeom = new THREE.BufferGeometry();
    belowGeom.setAttribute("position", position);
    belowGeom.setAttribute("color", color);
    const currentGeom = new THREE.BufferGeometry();
    currentGeom.setAttribute("position", position);
    currentGeom.setAttribute("color", color);
    const belowMat = new THREE.LineBasicMaterial({ vertexColors: true });
    belowMat.onBeforeCompile = (shader) => {
      shader.uniforms.fade = this.fade;
      shader.fragmentShader = shader.fragmentShader
        .replace("void main() {", "uniform vec3 fade;\nvoid main() {")
        .replace("#include <color_fragment>", "#include <color_fragment>\n\tdiffuseColor.rgb = mix(diffuseColor.rgb, fade, 0.86);");
    };
    this.below = new THREE.LineSegments(belowGeom, belowMat);
    this.current = new THREE.LineSegments(currentGeom, new THREE.LineBasicMaterial({ vertexColors: true }));
    this.current.renderOrder = 1;

    // Travels, drawn per layer.
    const m = tp.header.travels;
    const tpos = new Float32Array(m * 6);
    for (const layer of layers) {
      for (let i = layer.travel_start; i < layer.travel_start + layer.travel_count; i++) {
        const s = i * 4;
        const o = i * 6;
        tpos[o] = tp.travels[s];
        tpos[o + 1] = tp.travels[s + 1];
        tpos[o + 2] = layer.z;
        tpos[o + 3] = tp.travels[s + 2];
        tpos[o + 4] = tp.travels[s + 3];
        tpos[o + 5] = layer.z;
      }
    }
    const travelGeom = new THREE.BufferGeometry();
    travelGeom.setAttribute("position", new THREE.BufferAttribute(tpos, 3));
    this.travel = new THREE.LineSegments(
      travelGeom,
      new THREE.LineBasicMaterial({ color: 0x7c8cff, transparent: true, opacity: 0.55 }),
    );

    for (const obj of [this.below, this.current, this.travel]) {
      obj.frustumCulled = false;
      this.stage.scene.add(obj);
    }
    this.bounds = new THREE.Box3(new THREE.Vector3(...min), new THREE.Vector3(...max));
    this.stage.updateGrid();
    this.stage.setView("iso");
    this.showLayer(layers.length - 1);
  }

  /** Shows every layer up to and including `index`, with `index` highlighted. */
  showLayer(index: number) {
    if (!this.tp || !this.below || !this.current || !this.travel) return;
    const layers = this.tp.header.layers;
    this.layer = Math.max(0, Math.min(layers.length - 1, index));
    const l = layers[this.layer];
    this.below.geometry.setDrawRange(0, l.extrusion_start * 2);
    this.current.geometry.setDrawRange(l.extrusion_start * 2, l.extrusion_count * 2);
    this.travel.geometry.setDrawRange(l.travel_start * 2, l.travel_count * 2);
    this.travel.visible = this.showTravels;
  }

  setTravels(on: boolean) {
    this.showTravels = on;
    if (this.travel) this.travel.visible = on;
  }

  setView(preset: ViewPreset) {
    this.stage.setView(preset);
  }

  setOrthographic(on: boolean) {
    this.stage.setOrthographic(on);
  }

  setBackground(color: string) {
    this.stage.setBackground(color);
    this.fade.value.set(color);
  }

  setGrid(on: boolean) {
    this.stage.setGrid(on);
  }

  screenshot() {
    return this.stage.screenshot();
  }

  private clear() {
    for (const obj of [this.below, this.current, this.travel]) {
      if (!obj) continue;
      this.stage.scene.remove(obj);
      obj.geometry.dispose();
      (obj.material as THREE.Material).dispose();
    }
    this.below = this.current = this.travel = null;
  }

  dispose() {
    this.clear();
    this.stage.dispose();
  }
}
