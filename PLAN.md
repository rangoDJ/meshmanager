# MeshManager: Plan for a Self-Hosted 3D Model Library

A self-hosted web app, deployed with Docker Compose, that indexes the folders where your 3D print files already live (local disks, NAS mounts). It renders a thumbnail for every model, opens models in a browser 3D viewer, finds duplicates, and lets you search, tag and organize everything. It never moves, renames or uploads your files.

Feature set modeled on [Meshory](https://meshory.com/features), a paid desktop app, with changes where a server and browser setup works differently.

---

## 1. Guiding principles

| Principle | What it means here |
|---|---|
| **Read-only on your files** | Library folders are mounted into the container `:ro`. The app keeps its index, thumbnails and caches in its own `/data` volume and never writes into a library folder. |
| **Index in place** | You add folders as "sources". Nothing gets imported or copied. |
| **Scales to terabytes** | Scanning, hashing and rendering run as background jobs that can resume after interruption. The UI stays fast at 100k+ models (virtualized grid, paginated API, DB indexes). |
| **Private by default** | No telemetry and no cloud calls. AI features are opt-in and use your own key or a local Ollama. |
| **Metadata follows the file** | Tags and notes are tied to file content (hash), not only the path, so they survive renames and moves on disk. |

---

## 2. Feature list (parity map)

Legend: **MVP** = phase 1–2, **v1** = phase 3–5, **Later** = phase 6+.

### 2.1 Library and scanning
| Feature | Meshory | Self-hosted version | Phase |
|---|---|---|---|
| Multiple sources (folders, drives, NAS) | ✅ | Each mounted volume path can be added as a source in the UI | MVP |
| Background indexing of very large libraries | ✅ | Job queue with workers; progress bar per source; resumes after a restart | MVP |
| Incremental rescans | ✅ | Compare mtime and size; rehash only changed files; scheduled rescans plus an optional filesystem watcher (inotify for local disks; NFS/SMB fall back to polling) | MVP |
| Tags and notes follow renames and moves | ✅ | Match a missing file to a new file with the same content hash, then move its metadata over | v1 |
| Removing a source only clears the index | ✅ | Same | MVP |
| Photo-library-style grid | ✅ | Virtualized infinite grid, adjustable thumbnail size, list view | MVP |
| Filter by format, source, folder or tag | ✅ | Sidebar facets and a folder tree | MVP |
| Thumbnails as you browse, or all up front | ✅ | Per-source setting: "lazy" (render on request, priority queue) or "eager" (render all after a scan) | MVP |

### 2.2 Formats
| Group | Formats | Thumbnail | 3D view | Dedupe | Phase |
|---|---|---|---|---|---|
| Core meshes | STL, 3MF, OBJ | ✅ | ✅ | ✅ | MVP |
| Interchange | PLY, GLB, glTF | ✅ | ✅ | ✅ | MVP |
| Other mesh | FBX | ✅ | ✅ | ✅ | v1 |
| CAD | STEP/STP | ✅ (tessellated) | ✅ with part names | STEP vs STEP only | v1 |
| CAD | Rhino 3DM | ✅ (saved render meshes) | ✅ | ✅ | Later |
| CAD | Fusion F3D/F3Z | Embedded preview image | Best effort | — | Later |
| Resin | Lychee LYS | Experimental | Experimental | — | Later |
| Archives | ZIP, RAR (incl. split sets), 7Z, TAR, TAR.GZ, TGZ | Every model inside gets its own thumbnail | ✅ (extracted to cache on demand) | ✅ | MVP (ZIP) / v1 (rest) |
| Laser/CNC | xTool XCS/XS, LightBurn LBRN/LBRN2, DXF, DWG | Embedded preview, or a 2D render for DXF | 2D full-size view | — | Later |
| Companion files | JPG, PNG, GIF, WEBP, BMP, HEIC/HEIF, AVIF, SVG, PDF, TXT, MD, video | ✅ | Opens in place (Markdown rendered, PDF viewer, video player; video indexing off by default) | — | v1 |

### 2.3 3D viewer (browser, three.js)
- Rotate, pan, zoom; seven camera presets (front, back, left, right, top, bottom, isometric); auto-spin. **MVP**
- Multi-plate Bambu/Orca 3MF: plate selector, every part in place with its transforms, filament colors from the project settings. **MVP**
- STEP assemblies with a part tree of names. **v1**
- Model color remembered per file; PNG screenshot. **MVP**
- Info panel: dimensions (X/Y/Z), mesh volume, surface area, triangle count, watertight check, estimated print weight (pick a material density and infill %). **MVP**
- Roadmap items: measure distance between two points and the angle between three, background color, perspective/orthographic toggle. **v1**

### 2.4 Duplicate finder
- **Exact copies:** same file bytes (BLAKE3 hash). **MVP**
- **Identical geometry:** same mesh in a different file or format (STL vs OBJ vs 3MF), using a canonical geometry hash. **v1**
- **Likely duplicates (needs review):** similar shape descriptor (sorted bounding box, volume, area, triangle count, D2 shape histogram) plus a thumbnail perceptual hash plus filename similarity. **v1**
- Each group suggests which copy to keep, with rules such as: prefer files outside archives, prefer cleaner names (no `(1)`, `copy`, `kopie`), prefer 3MF over STL. **v1**
- Never deletes anything. Offers "export list" (CSV) and "mark as ignored". **v1**

### 2.5 Organization
- Tags (manual and bulk), notes per model, star/favorite. **v1**
- Collections that span folders, plus a "Not in a collection" view. **v1**
- Upload your own photos to a model (for example, a photo of the print), stored in `/data`. **v1**
- Print queue: multiple lists (such as "market run" or "gifts"), with each item marked queued, printed or failed, and export to PDF or CSV. **v1**

### 2.6 Search
- Instant search as you type by name, path, tag or notes (Postgres full-text plus trigram fuzzy match). **MVP**
- Keyboard shortcut (`/` or `Ctrl+K`) for a command palette. **MVP**
- Semantic search by meaning ("articulated dragon", "cable clip") using embeddings stored in pgvector. **v1**

### 2.7 AI tagging (opt-in, bring your own key)
- Providers: OpenAI-compatible API (OpenAI, OpenRouter), Anthropic, Gemini, local Ollama. **v1**
- Sends one or more thumbnails plus the filename and path to a vision model, which returns tags and a short description. Runs only when you start it (per selection, per folder, or the whole library), shows a cost estimate, and can be rate limited. **v1**
- Embeddings for semantic search from the same provider or from Ollama (`nomic-embed-text`). **v1**

### 2.8 Getting files out (replacing desktop-only features)
Meshory is a desktop app, so some features need a web equivalent:

| Desktop feature | Web replacement | Phase |
|---|---|---|
| Open in slicer (16+ slicers detected) | **Open in slicer** buttons using slicer URL schemes such as `bambustudio://open?file=<url>`, `orcaslicer://open?file=<url>` and `prusaslicer://open?file=<url>`. They point at a short-lived signed download URL. Plain download always works too. | v1 |
| Reveal in Finder/Explorer | Show and copy the host path (with a per-source "host path prefix" setting so the path matches your machine or SMB share) | MVP |
| Drag into any app | Download a single file, or a multi-select ZIP that streams on the fly | MVP |
| Global OS shortcut | In-app shortcuts, plus installable PWA | v1 |
| Send to xTool / Open in LightBurn | Download, or a URL scheme where the vendor supports one | Later |

### 2.9 Backup and restore
- Export all user data (tags, collections, notes, print queues, uploaded images, settings) to one `.tar.zst` file, and restore it on a new install. **v1**
- Scheduled backups to `/data/backups` with retention (a Meshory roadmap item). **v1**
- The index and thumbnails are *not* backed up, because they can be rebuilt with a rescan.

### 2.10 Extras that come from self-hosting (beyond Meshory)
- **Accounts:** local users with admin, editor and viewer roles; optional OIDC (Authentik, Keycloak, Authelia); or "trust reverse-proxy header" mode. **MVP (single admin) / v1 (roles, OIDC)**
- Read-only share links for a model or collection. **Later**
- REST API with API tokens and OpenAPI docs. **v1**
- Prometheus `/metrics` and a health endpoint. **v1**
- 3MF converter between slicer flavors (Bambu ⇄ Orca ⇄ Prusa and others). This is large, so treat it as **stretch**.

---

## 3. Architecture

```
                ┌──────────────── Browser (React + three.js) ────────────────┐
                │ grid · viewer · search · duplicates · collections · queue  │
                └───────────────────────────┬────────────────────────────────┘
                                            │ HTTPS (behind your reverse proxy)
┌──────────────────────── docker compose ───┴─────────────────────────────────┐
│  app      Go binary `serve`: REST API + embedded frontend + file streaming    │
│  worker   same image `worker`, N replicas: scan · hash · thumbnail · geometry │
│  db       PostgreSQL 16 + pgvector + pg_trgm; also holds the River job queue  │
│  (convert) optional sidecar for STEP/FBX/3DM/HEIC/PDF/video conversions       │
│  (ollama) optional, for local AI tagging/embeddings                           │
│                                                                               │
│  volumes:  /libraries/*  (your files, read-only)                              │
│            /data         (thumbnails, mesh cache, uploads, backups)           │
└───────────────────────────────────────────────────────────────────────────────┘
```

### 3.1 Tech stack
| Layer | Choice | Why |
|---|---|---|
| Backend | **Go 1.27**, stdlib `net/http` routing, `log/slog`, **pgx v5**, **goose** SQL migrations (embedded) | One static binary with subcommands (`serve`, `worker`, `migrate`, `reset-password`, `healthcheck`) |
| Job queue | **River**: the job queue lives in Postgres, with transactional enqueue, retries and periodic jobs; plus a `jobs` table for user-facing progress | No Redis. A job and its tracking row commit together. |
| Database | **PostgreSQL 16** with `pg_trgm` and `pgvector` | One database covers relational data, fuzzy search, vector search and the queue |
| Auth | argon2id passwords, random session tokens (only their SHA-256 is stored) in an HttpOnly cookie, CSRF header guard, login rate limiting | |
| Mesh loading (Go) | Own parsers for STL (ASCII and binary), OBJ, PLY; `qmuntal/gltf` for GLB/glTF; 3MF via `archive/zip` + `encoding/xml` (including Bambu `model_settings.config` plates and filament colors) | Covers the common formats with no cgo |
| Thumbnail rendering | **`fogleman/fauxgl`**, a pure-Go software rasterizer (CPU, headless); embedded previews used first when present (3MF `Metadata/plate_*.png`, F3D, DWG, LightBurn) | Static binary, no GPU or OpenGL needed |
| Heavy formats | Optional **`convert` sidecar** (Python: OCP for STEP, assimp for FBX, rhino3dm, libheif, PyMuPDF, ffmpeg) behind a small HTTP API that returns GLB or PNG | Keeps the main image small; enable only if you need these formats |
| Archives | stdlib `archive/zip`, `archive/tar`, `compress/gzip`; `bodgit/sevenzip`; `nwaples/rardecode` (pure Go, multi-volume) | Reads entries in place; members are extracted to `/data/cache` only on demand |
| Images | stdlib + `golang.org/x/image` (PNG/JPG/GIF/WEBP/BMP); HEIC/AVIF/PDF/video through the sidecar | |
| Hashing | BLAKE3 (`zeebo/blake3`, SIMD) | Fast on large files |
| Frontend | **React 19 + TypeScript + Vite**, TanStack Query, React Router, Tailwind 4; later `react-virtuoso` grid and **three.js via react-three-fiber + drei** | Built into `dist/` and embedded in the Go binary |
| Viewer data | The server converts any model to a cached **GLB** (plus a plate and part manifest JSON). The browser only ever loads GLB. | One viewer code path; STEP and FBX tessellation happen once |
| Packaging | Multi-stage Dockerfile (node build → go build → distroless/static, about 20 MB); amd64 and arm64 | Runs on a Raspberry Pi 5 or ARM NAS too |

### 3.2 Data model (core tables)
```
source(id, name, container_path, host_path_prefix, mode[lazy|eager], scan_schedule, enabled)
folder(id, source_id, rel_path, parent_id)
file(id, source_id, folder_id, rel_path, archive_parent_id NULL, archive_member_path NULL,
     kind[model|archive|image|doc|video|laser], format, size, mtime,
     content_hash, geometry_hash NULL, status[indexed|missing|error], first_seen, last_seen)
model_info(file_id, bbox_x, bbox_y, bbox_z, volume_mm3, area_mm2, triangles, watertight,
           plates_json, parts_json, colors_json, shape_descriptor vector, phash)
thumbnail(file_id, variant[grid|large|plate_n], path, rendered_at, renderer_version)
asset(content_hash PK)          ← user data hangs off content, so it survives renames
  asset_tag(content_hash, tag_id) · asset_note(content_hash, body) · asset_photo(...)
tag(id, name, color, source[manual|ai])
collection(id, name, ...) · collection_item(collection_id, content_hash)
print_queue(id, name) · print_queue_item(queue_id, content_hash, qty, status, note)
dup_group(id, kind[exact|geometry|likely], suggested_keep_file_id, reviewed, ignored)
dup_member(group_id, file_id)
ai_description(content_hash, provider, model, text, tags_json, embedding vector, created_at)
job(id, type, payload, state, progress, error) · user(...) · api_token(...) · setting(k, v)
```

### 3.3 Scan pipeline
1. **Walk:** stream directory entries with `filepath.WalkDir`, insert or update `file` rows in batches of about 1,000, and mark rows not seen as `missing`.
2. **Hash:** BLAKE3 over files whose size or mtime changed (with an I/O concurrency limit that is friendly to NAS drives).
3. **Reconcile:** a `missing` file whose hash now appears at a new path gets its old ID reused, so user metadata never orphans.
4. **Archive expand:** list members and create child `file` rows (no extraction).
5. **Analyze (models):** load the mesh, compute stats, the geometry hash and the shape descriptor; for 3MF, parse plates and colors.
6. **Thumbnail:** use the embedded preview if present, otherwise render. Lazy mode queues only when the grid requests a thumbnail; eager mode queues everything.
7. **Dedupe pass:** group by `content_hash`, then by `geometry_hash`, then run a vector nearest-neighbor search on the shape descriptor for "likely" candidates.

Geometry hash: load the mesh, merge vertices, quantize to 1e-4 mm, sort each triangle's vertices, sort the triangle list, then hash. This ignores file format, vertex order and triangle order. A rotation/translation-invariant variant (centering on the centroid and aligning to principal axes) can come later.

---

## 4. Deployment

See [`docker-compose.yml`](docker-compose.yml) and the [README](README.md). It defines three services: `app` (`serve`), `worker` (`worker`, scalable) and `db` (pgvector/pg16).
- Runs as a non-root user (`PUID`/`PGID`) in a distroless image; `app` runs migrations on start, and `worker` waits for `app` to be healthy.
- Library folders are mounted `:ro` under `/libraries/*`.
- Put it behind Caddy, Traefik or Nginx Proxy Manager for TLS (`TRUST_PROXY=true`). Optional `convert` and `ollama` services come later as compose profiles.

---

## 5. Project layout
```
meshmanager/
├─ backend/                    # Go module "meshmanager"
│  ├─ cmd/meshmanager/         # entry point and subcommands            ✅ M0
│  └─ internal/
│     ├─ api/                  # HTTP handlers + middleware              ✅ M0 (auth, jobs, system)
│     ├─ auth/                 # argon2id, tokens, rate limiter          ✅ M0
│     ├─ config/               # env config                              ✅ M0
│     ├─ db/migrations/        # goose SQL migrations                    ✅ M0
│     ├─ jobs/                 # River job args + workers                ✅ M0 (ping, session cleanup)
│     ├─ store/                # SQL queries                             ✅ M0
│     ├─ web/                  # embedded frontend                       ✅ M0
│     ├─ scan/                 # walker, hasher, reconcile               M1
│     ├─ formats/              # one package per format: detect, analyze, to GLB, embedded preview
│     ├─ render/               # fauxgl thumbnailer
│     ├─ archives/             # zip/tar/7z/rar readers, member cache
│     ├─ dedupe/               # geometry hash, shape descriptors, keeper rules
│     └─ ai/                   # provider adapters
├─ frontend/src/{pages,components,api}                                 ✅ M0
├─ convert/                    # optional Python sidecar (later)
├─ Dockerfile · docker-compose.yml · docker-compose.dev.yml           ✅ M0
├─ .github/workflows/ci.yml                                            ✅ M0
└─ PLAN.md · README.md
```

---

## 6. Roadmap / milestones

| # | Milestone | Deliverables | Done when |
|---|---|---|---|
| 0 ✅ | **Skeleton** | Repo, Dockerfile, compose, Go API + React shell, Postgres + goose, River worker, single-admin login, CI (lint, test, multi-arch build) | `docker compose up` shows the login page and a health check passes |
| 1 | **Index and grid (MVP core)** | Sources UI, scanner, BLAKE3 hashing, STL/OBJ/3MF/PLY/GLB analysis, fauxgl thumbnails, lazy/eager modes, virtualized grid, folder tree, format filter, name/path search, download | A 100k-file test library indexes, and scrolling stays smooth |
| 2 | **Viewer** | GLB conversion cache, three.js viewer, camera presets, Bambu multi-plate and colors, info panel (dimensions/volume/weight), screenshots, ZIP browsing | A Bambu multi-plate 3MF shows the right plates and colors |
| 3 | **Archives, formats, companion files** | RAR/7Z/TAR, `convert` sidecar (FBX, STEP, HEIC, PDF, video), images/PDF/MD/video viewer, host-path "reveal" | Every format in §2.2 marked MVP/v1 works |
| 4 | **Duplicates** | Exact, geometry and likely groups, keeper suggestions, review UI, CSV export | A seeded test set produces the expected groups |
| 5 | **Organize** | Tags, notes, collections, "Not in a collection", photos, print queue with PDF/CSV export, rename/move reconciliation, bulk actions | Tags survive a rename on disk followed by a rescan |
| 6 | **AI and semantic search** | Provider settings, tagging jobs with cost estimates, embeddings, semantic search | "dragon" finds `untitled(3).stl` when it is a dragon |
| 7 | **Polish and ops** | Backup/restore with a schedule, slicer URL-scheme buttons, PWA, users/roles, OIDC, API tokens, metrics, viewer measuring tools | Restore on a fresh install brings back all user data |
| 8 | **Later / stretch** | 3DM, F3D, LYS, laser/CNC previews, share links, 3MF slicer converter, GPU rendering | — |

---

## 7. Risks and open questions
- **Thumbnail throughput on CPU:** a software rasterizer takes roughly 0.1–1 s per model, depending on triangle count. Mitigations: lazy mode, embedded previews first, decimating very large meshes before rendering, and multiple worker replicas.
- **Huge meshes (100 MB+ STLs) in the browser:** decimate to a viewer LOD on the server and offer "load full resolution".
- **NAS over SMB/NFS:** no inotify, and file reads are slow. Rely on mtime/size polling and throttle I/O concurrency per source.
- **No Go CAD kernel:** STEP, FBX and 3DM need the optional Python `convert` sidecar (OCP adds hundreds of MB), which keeps the main image small.
- **RAR:** `nwaples/rardecode` is pure Go and handles RAR4/RAR5 and split sets; very rare edge cases may still need `unrar` in the sidecar.
- **Slicer URL schemes:** support varies by slicer and version. Plain download must always work.
- **Decision needed:** single-user or multi-user from day one? (The plan assumes a single admin first, with the schema ready for multiple users.)
