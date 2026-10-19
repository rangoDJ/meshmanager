import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import type { Geometry } from "./geometry";

export type ViewPreset = "iso" | "front" | "back" | "left" | "right" | "top" | "bottom";

// Directions from the model towards the camera. Z is up and the front is -Y, as in slicers.
// Top and bottom lean slightly towards the front so the orbit has a stable "up".
const VIEW_DIRS: Record<ViewPreset, [number, number, number]> = {
  iso: [1, -1.4, 1.1],
  front: [0, -1, 0],
  back: [0, 1, 0],
  left: [-1, 0, 0],
  right: [1, 0, 0],
  top: [0, -1e-4, 1],
  bottom: [0, -1e-4, -1],
};

const FOV = 35;

/**
 * A small three.js scene for inspecting one model: orbit controls, a headlight that
 * follows the camera, an optional build-plate grid, and one mesh per model part so
 * plates and parts can be shown or hidden.
 */
export class ModelScene {
  private renderer: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private perspective: THREE.PerspectiveCamera;
  private ortho: THREE.OrthographicCamera;
  private camera: THREE.PerspectiveCamera | THREE.OrthographicCamera;
  private controls: OrbitControls;
  private key = new THREE.DirectionalLight(0xffffff, 2.2);
  private grid: THREE.GridHelper | null = null;
  private meshes: THREE.Mesh[] = [];
  private materials: THREE.MeshStandardMaterial[] = [];
  private partColors: (string | undefined)[] = [];
  private defaultColor = "#a3b1d6";
  private lastPreset: ViewPreset = "iso";
  private resizeObserver: ResizeObserver;
  private showGrid = true;

  constructor(private container: HTMLElement) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: false });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    container.appendChild(this.renderer.domElement);
    this.renderer.domElement.style.display = "block";

    this.perspective = new THREE.PerspectiveCamera(FOV, 1, 0.1, 10000);
    this.ortho = new THREE.OrthographicCamera(-1, 1, 1, -1, 0.1, 10000);
    for (const cam of [this.perspective, this.ortho]) cam.up.set(0, 0, 1);
    this.camera = this.perspective;

    this.scene.add(new THREE.HemisphereLight(0xffffff, 0x404050, 1.1));
    this.scene.add(this.key, this.key.target);

    this.controls = this.makeControls(this.camera);
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(container);
    this.resize();

    this.renderer.setAnimationLoop(() => {
      this.controls.update();
      // Headlight from just above and left of the camera.
      this.key.position.copy(this.camera.position).add(new THREE.Vector3(0, 0, 0.3).multiplyScalar(this.radius()));
      this.key.target.position.copy(this.controls.target);
      this.renderer.render(this.scene, this.camera);
    });
  }

  private makeControls(camera: THREE.Camera) {
    const c = new OrbitControls(camera, this.renderer.domElement);
    c.enableDamping = true;
    c.dampingFactor = 0.12;
    c.screenSpacePanning = true;
    return c;
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
      mesh.userData.plate = part.plate;
      this.meshes.push(mesh);
      this.materials.push(mat);
      this.scene.add(mesh);
    }
    this.updateGrid();
    this.setView("iso");
  }

  private clearModel() {
    for (const m of this.meshes) {
      this.scene.remove(m);
      m.geometry.dispose();
    }
    for (const m of this.materials) m.dispose();
    this.meshes = [];
    this.materials = [];
  }

  /** Shows only the parts for which visible(index) is true, then reframes. */
  setVisibleParts(visible: (index: number) => boolean, reframe = true) {
    this.meshes.forEach((m, i) => (m.visible = visible(i)));
    this.updateGrid();
    if (reframe) this.setView(this.lastPreset);
  }

  private visibleBox() {
    const box = new THREE.Box3();
    for (const m of this.meshes) if (m.visible && m.geometry.boundingBox) box.union(m.geometry.boundingBox);
    if (box.isEmpty()) box.set(new THREE.Vector3(-10, -10, 0), new THREE.Vector3(10, 10, 20));
    return box;
  }

  private radius() {
    return this.visibleBox().getBoundingSphere(new THREE.Sphere()).radius || 10;
  }

  setView(preset: ViewPreset) {
    this.lastPreset = preset;
    const sphere = this.visibleBox().getBoundingSphere(new THREE.Sphere());
    const r = Math.max(sphere.radius, 1);
    const dir = new THREE.Vector3(...VIEW_DIRS[preset]).normalize();
    const dist = (r / Math.sin(THREE.MathUtils.degToRad(FOV / 2))) * 1.08;

    this.perspective.position.copy(sphere.center).addScaledVector(dir, dist);
    this.perspective.near = dist / 200;
    this.perspective.far = dist * 50;
    this.perspective.updateProjectionMatrix();

    this.ortho.position.copy(sphere.center).addScaledVector(dir, r * 4);
    this.ortho.near = 0.01;
    this.ortho.far = r * 20;
    this.ortho.zoom = 1;
    this.orthoHalfHeight = r * 1.08;
    this.updateOrthoFrustum();

    this.controls.target.copy(sphere.center);
    this.controls.update();
  }

  private orthoHalfHeight = 50;

  private updateOrthoFrustum() {
    const { clientWidth: w, clientHeight: h } = this.container;
    const aspect = w / Math.max(h, 1);
    this.ortho.top = this.orthoHalfHeight;
    this.ortho.bottom = -this.orthoHalfHeight;
    this.ortho.left = -this.orthoHalfHeight * aspect;
    this.ortho.right = this.orthoHalfHeight * aspect;
    this.ortho.updateProjectionMatrix();
  }

  setOrthographic(on: boolean) {
    const next = on ? this.ortho : this.perspective;
    if (next === this.camera) return;
    const target = this.controls.target.clone();
    const dir = this.camera.position.clone().sub(target).normalize();
    this.camera = next;
    this.controls.dispose();
    this.controls = this.makeControls(this.camera);
    // Keep the current viewing direction.
    const r = this.radius();
    const dist = on ? r * 4 : (r / Math.sin(THREE.MathUtils.degToRad(FOV / 2))) * 1.08;
    this.camera.position.copy(target).addScaledVector(dir, dist);
    this.controls.target.copy(target);
    this.controls.update();
  }

  setAutoRotate(on: boolean) {
    this.controls.autoRotate = on;
    this.controls.autoRotateSpeed = 2;
  }

  setWireframe(on: boolean) {
    for (const m of this.materials) m.wireframe = on;
  }

  /** Colour for parts whose file doesn't specify one. */
  setDefaultColor(color: string) {
    this.defaultColor = color;
    this.materials.forEach((m, i) => {
      if (!this.partColors[i]) m.color.set(color);
    });
  }

  /** Forces every part to one colour, or restores file colours with null. */
  setOverrideColor(color: string | null) {
    this.materials.forEach((m, i) => m.color.set(color ?? this.partColors[i] ?? this.defaultColor));
  }

  setBackground(color: string) {
    this.renderer.setClearColor(color);
    this.updateGrid();
  }

  setGrid(on: boolean) {
    this.showGrid = on;
    this.updateGrid();
  }

  private updateGrid() {
    if (this.grid) {
      this.scene.remove(this.grid);
      this.grid.geometry.dispose();
      (this.grid.material as THREE.Material).dispose();
      this.grid = null;
    }
    if (!this.showGrid || this.meshes.length === 0) return;
    const box = this.visibleBox();
    const size = box.getSize(new THREE.Vector3());
    const center = box.getCenter(new THREE.Vector3());
    // 10 mm squares, at least a 200 mm plate, snapped to whole centimetres.
    const extent = Math.max(200, Math.ceil((Math.max(size.x, size.y) * 1.4) / 10) * 10);
    const clear = this.renderer.getClearColor(new THREE.Color());
    const light = clear.getHSL({ h: 0, s: 0, l: 0 }).l > 0.5;
    const grid = new THREE.GridHelper(extent, extent / 10, light ? 0x9a9aa6 : 0x55555f, light ? 0xcfcfd6 : 0x2c2c33);
    grid.rotation.x = Math.PI / 2; // GridHelper lies in XZ; turn it into the XY build plate
    grid.position.set(Math.round(center.x / 10) * 10, Math.round(center.y / 10) * 10, box.min.z - 0.01);
    this.grid = grid;
    this.scene.add(grid);
  }

  async screenshot(): Promise<Blob> {
    this.renderer.render(this.scene, this.camera);
    return new Promise((resolve, reject) =>
      this.renderer.domElement.toBlob((b) => (b ? resolve(b) : reject(new Error("Screenshot failed"))), "image/png"),
    );
  }

  private resize() {
    const { clientWidth: w, clientHeight: h } = this.container;
    if (!w || !h) return;
    this.renderer.setSize(w, h, false);
    this.renderer.domElement.style.width = "100%";
    this.renderer.domElement.style.height = "100%";
    this.perspective.aspect = w / h;
    this.perspective.updateProjectionMatrix();
    this.updateOrthoFrustum();
  }

  dispose() {
    this.renderer.setAnimationLoop(null);
    this.resizeObserver.disconnect();
    this.controls.dispose();
    this.clearModel();
    this.renderer.dispose();
    this.renderer.domElement.remove();
  }
}
