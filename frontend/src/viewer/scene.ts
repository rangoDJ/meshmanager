import * as THREE from "three";
import type { Geometry } from "./geometry";
import { Stage, type ViewPreset } from "./stage";

export type { ViewPreset };

/**
 * A model on the shared stage: one mesh per model part, so plates and parts can be
 * shown or hidden.
 */
export class ModelScene {
  private stage: Stage;
  private meshes: THREE.Mesh[] = [];
  private materials: THREE.MeshStandardMaterial[] = [];
  private partColors: (string | undefined)[] = [];
  private defaultColor = "#a3b1d6";

  constructor(container: HTMLElement) {
    this.stage = new Stage(container, () => this.visibleBox());
  }

  setModel(g: Geometry) {
    this.clearModel();
    this.partColors = g.header.parts.map((p) => p.color);
    for (const part of g.header.parts) {
      const geom = new THREE.BufferGeometry();
      const positions = g.positions.subarray(part.start * 9, (part.start + part.count) * 9);
      geom.setAttribute("position", new THREE.BufferAttribute(positions, 3));
      geom.computeVertexNormals(); // non-indexed, so these are face normals: a crisp, faceted look
      geom.computeBoundingBox();
      const mat = new THREE.MeshStandardMaterial({
        color: part.color ?? this.defaultColor,
        roughness: 0.55,
        metalness: 0.05,
        side: THREE.DoubleSide,
      });
      const mesh = new THREE.Mesh(geom, mat);
      this.meshes.push(mesh);
      this.materials.push(mat);
      this.stage.scene.add(mesh);
    }
    this.stage.updateGrid();
    this.stage.setView("iso");
  }

  private clearModel() {
    for (const m of this.meshes) {
      this.stage.scene.remove(m);
      m.geometry.dispose();
    }
    for (const m of this.materials) m.dispose();
    this.meshes = [];
    this.materials = [];
  }

  private visibleBox() {
    const box = new THREE.Box3();
    for (const m of this.meshes) if (m.visible && m.geometry.boundingBox) box.union(m.geometry.boundingBox);
    return box.isEmpty() ? null : box;
  }

  /** Shows only the parts for which visible(index) is true, then reframes. */
  setVisibleParts(visible: (index: number) => boolean, reframe = true) {
    this.meshes.forEach((m, i) => (m.visible = visible(i)));
    this.stage.updateGrid();
    if (reframe) this.stage.setView();
  }

  setView(preset: ViewPreset) {
    this.stage.setView(preset);
  }

  setOrthographic(on: boolean) {
    this.stage.setOrthographic(on);
  }

  setAutoRotate(on: boolean) {
    this.stage.setAutoRotate(on);
  }

  setWireframe(on: boolean) {
    for (const m of this.materials) m.wireframe = on;
  }

  /** Forces every part to one colour, or restores file colours with null. */
  setOverrideColor(color: string | null) {
    this.materials.forEach((m, i) => m.color.set(color ?? this.partColors[i] ?? this.defaultColor));
  }

  setBackground(color: string) {
    this.stage.setBackground(color);
  }

  setGrid(on: boolean) {
    this.stage.setGrid(on);
  }

  screenshot() {
    return this.stage.screenshot();
  }

  dispose() {
    this.clearModel();
    this.stage.dispose();
  }
}
