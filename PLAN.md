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
│  app      Go binary `serve`: REST API + embedded frontend + background jobs  │
│  (worker) optional extra job containers (`worker`) for very large libraries   │
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
| Thumbnail rendering | Own pure-Go software rasterizer (`internal/render`: orthographic z-buffer, two-sided Lambert, 2× supersampling) working directly on the flat triangle array, about 36 bytes per triangle; 1M triangles in roughly 150 ms per core; embedded previews used first when present (3MF `Metadata/plate_*.png`, F3D, DWG, LightBurn) | Static binary, no GPU or OpenGL needed |
| Heavy formats | Optional **`convert` sidecar** (Python: OCP for STEP, assimp for FBX, rhino3dm, libheif, PyMuPDF, ffmpeg) behind a small HTTP API that returns GLB or PNG | Keeps the main image small; enable only if you need these formats |
| Archives | stdlib `archive/zip`, `archive/tar`, `compress/gzip`; `bodgit/sevenzip`; `nwaples/rardecode` (pure Go, multi-volume) | Reads entries in place; members are extracted to `/data/cache` only on demand |
| Images | stdlib + `golang.org/x/image` (PNG/JPG/GIF/WEBP/BMP); HEIC/AVIF/PDF/video through the sidecar | |
| Hashing | BLAKE3 (`zeebo/blake3`, SIMD) | Fast on large files |
| Frontend | **React 19 + TypeScript + Vite**, TanStack Query, React Router, Tailwind 4; later `react-virtuoso` grid and **three.js via react-three-fiber + drei** | Built into `dist/` and embedded in the Go binary |
| Viewer data | `GET /api/files/{id}/geometry` converts any model on demand to a compact binary: a JSON header (parts with names, plates and colours; bounds; volume; watertight) followed by `float32` triangles. It's gzipped, and the browser caches it via the ETag. three.js renders one mesh per part. | One viewer code path through the tested Go loaders; no conversion cache filling up `/data` |
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

See [`docker-compose.yml`](docker-compose.yml) and the [README](README.md). By default it runs two services: `app` (`serve`: web UI, API and background jobs in one process) and `db` (pgvector/pg16). An optional `worker` profile adds extra job containers.
- App settings (thumbnail size, rescan interval, hashing parallelism, video indexing) are edited in the UI and stored in `DATA_DIR/config.json`; environment variables cover deployment only.
- Runs as a non-root user (`PUID`/`PGID`) in a distroless image; `app` runs migrations on start.
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
│     ├─ scan/                 # walker, BLAKE3 hasher                    ✅ M1
│     ├─ formats/              # extension → kind/format table            ✅ M1
│     ├─ mesh/                 # STL/OBJ/PLY/3MF/GLB loaders + stats      ✅ M1
│     ├─ thumbs/               # thumbnail generation (render, embedded, images) ✅ M1
│     ├─ render/               # software rasterizer for thumbnails  ✅ M1
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

Revised 2026-10-19 after reviewing Meshory's public roadmap (see §8). Milestones 0–2 are done; the rest are reordered and extended.

| # | Milestone | Deliverables | Done when |
|---|---|---|---|
| 0 ✅ | **Skeleton** | Repo, Dockerfile, compose, Go API + React shell, Postgres + goose, River jobs, single-admin login, CI (lint, test, multi-arch build) | `docker compose up` shows the login page and a health check passes |
| 1 ✅ | **Index and grid (MVP core)** | Sources UI, scanner, BLAKE3 hashing, STL/OBJ/3MF/PLY/GLB analysis, software-rendered thumbnails, lazy/eager modes, virtualized grid, folder tree, format filter, name/path search, download | A 100k-file test library indexes, and scrolling stays smooth |
| 2 ✅ | **Viewer** | On-demand geometry endpoint, three.js viewer, camera presets, Bambu/Orca plates and colours, PrusaSlicer volumes, info panel (dimensions/volume/weight/watertight), screenshots, ZIP browsing | A Bambu multi-plate 3MF shows the right plates and colours |
| 3 ✅ | **Archives, companion files, light formats** | RAR (split sets listed once, on the first part), 7Z, TAR/TGZ; viewer for images (pixel size, DPI and printed size), SVG, PDF (browser), TXT/Markdown readmes, video playback; SVG thumbnails; glTF; previews embedded in G-code, FreeCAD `.FCStd` and LightBurn files; **G-code layer preview** (open a sliced file and step through the print layer by layer: toolpaths, travel moves, layer height and time estimates where the slicer saved them); per-source host path with copy-location | Every archive type browses like a folder, every companion file opens in the app, and a sliced G-code file can be stepped through layer by layer |
| 4 | **Duplicates** | Exact, identical-geometry and likely groups; folder duplicates; "one pack inside another" verdict with what each side would lose; ZIP next to its unpacked folder rolled up; keeper suggestions; source name and modified time per copy; bulk review (ignore or accept many groups in one pass); CSV export | A seeded test set produces the expected groups, including folder and pack-in-pack cases |
| 5 | **Scanning controls** | Exclude folders (ghost cards, restore), include patterns and file types, cleanup list of folders with no models, pause/resume and enable/disable per source, rescan one folder, rescan from scratch, scan issues list with retry/ignore, offline-source banner | Excluding a folder hides it without touching disk, and a failed archive can be retried from the UI |
| 6 ✅ | **Organize** | Tags with pinned colour dots, multi-tag filter (all/any, minus to exclude), notes, nested and smart collections (auto-filing rules), "Not in a collection", provenance (source URL, designer, licence), custom thumbnails and folder covers, uploaded photos, print queue with lists and per-printer boards plus PDF/CSV export (CSV is a download; PDF is the browser's Save as PDF from a print view), rename/move reconciliation (by content hash, or size and time for pictures and documents), bulk actions (tag, collection, print queue; on a selection or on everything a filter matches) | Tags survive a rename on disk followed by a rescan |
| 7 | **Model sets** | A figure that comes as many parts (e.g. thirteen STLs) shows as one card, not thirteen. Sets are suggested when a folder holds a model's parts (shared folder or name stem, a pack's STL folder) and confirmed or dismissed in one click, or built by hand from files anywhere in the library; every part stays a normal file you can open, view in 3D and download from inside the set; a set card shows a cover (chosen part or composite of the parts) and part count; download the whole set as one ZIP; sets work with search, tags, collections and the print queue | A folder with a 13-part figure shows one suggested set card that opens to its 13 parts, each still viewable on its own |
| 8 | **AI, semantic search and MCP** | Provider settings (OpenAI-compatible, Anthropic, Gemini, Ollama), your own tagging rules in plain words, tagging runs you can pause/stop with a live request and cost log, embeddings and semantic search, an MCP server exposing the library to AI assistants | "dragon" finds `untitled(3).stl` when it is a dragon |
| 9 | **Polish and ops** | Backup/restore with a schedule, slicer URL-scheme buttons and custom slicers, viewer measuring tools, hover quick look, studio-style thumbnails, light/dark/system theme switch, mm/inch units, help beside each setting, type-to-search, PWA, users/roles, OIDC, API tokens, metrics | Restore on a fresh install brings back all user data |
| 10 | **Filament inventory** | Spools with material, colour, brand, diameter, remaining and empty-spool weight, and cost; add, edit, weigh in and archive spools; prints marked done in the print queue deduct filament (from the slicer's G-code or 3MF figures, else the weight estimate) from the chosen spool; each model shows whether it's printable with what's on hand (enough of a matching material and colour per filament in multi-colour 3MFs), with a library filter for it; low-stock warnings; optional sync with a Spoolman server instead of MeshManager's own spool list | A 3MF that needs 120 g of red shows as not printable when the red spool has 80 g left, and marking a print as done deducts its filament |
| 11 | **Move and manage files (opt-in)** | **Move files and folders between folders**: sort a downloads folder from inside MeshManager (drag to a folder in the tree, or "Move to…"), within a source or across sources, and the library follows without a rescan (tags, notes, thumbnails and set membership move with the file); rename files and folders; trash with undo (to a trash folder inside the source); hide single files; trash from the viewer and from duplicate groups. Needs a per-source switch and a read-write mount, so it's off by default | Moving a model from Downloads into a sorted folder moves it on disk and in the library in one step, with its tags kept and no rescan |
| 12 | **Import from Patreon, Printables & MakerWorld** *(planned for later)* | Browser extension that saves a model into the library as you download it, or brings a whole Patreon release across with its pictures and attachments; creator, licence and source link attached automatically (uses M6 provenance fields); files land in a writable inbox folder (a separate read-write mount, since libraries are read-only) and are indexed right away; the extension authenticates with an API token (M9) | Downloading a model on Printables with the extension installed puts it in the library with its creator, licence and source link |
| 13 | **Later / stretch** | Heavy formats deferred on 2026-10-19 (STEP, FBX, 3DM, F3D, LYS, HEIC, PDF and video thumbnails), DXF/DWG/xTool previews, 3MF converter and profile library, model versioning, search inside nested archives, share links, translations, GPU rendering | — |

---

## 7. Risks and open questions
- **Thumbnail throughput on CPU:** a software rasterizer takes roughly 0.1–1 s per model, depending on triangle count. Mitigations: lazy mode, embedded previews first, decimating very large meshes before rendering, and multiple worker replicas.
- **Huge meshes (100 MB+ STLs) in the browser:** decimate to a viewer LOD on the server and offer "load full resolution".
- **NAS over SMB/NFS:** no inotify, and file reads are slow. Rely on mtime/size polling and throttle I/O concurrency per source.
- **No Go CAD kernel:** STEP, FBX and 3DM need native tools (OpenCascade, assimp). On 2026-10-19 these were deferred to M13 to keep a single small image; when picked up, the options are bundling them into the image or an optional sidecar.
- **RAR and 7Z:** `nwaples/rardecode/v2` (RAR4/RAR5, split sets) and `bodgit/sevenzip` are pure Go. Solid archives must be decompressed from the start to reach a member, so opening files deep inside large solid archives is slow.
- **Slicer URL schemes:** support varies by slicer and version. Plain download must always work.
- **Decision needed:** single-user or multi-user from day one? (The plan assumes a single admin first, with the schema ready for multiple users.)

---

## 8. Meshory roadmap review (2026-10-19)

Source: [meshory.com/roadmap](https://meshory.com/roadmap): 58 shipped, 8 in progress, 19 planned, plus community votes.

**Already covered here:** visual library and viewer, multi-plate 3MF with colours, viewer colours/views/screenshots, ZIP browsing, PLY and GLB, preview mode setting (lazy/eager), thumbnail sizes, a server deployment (Meshory only plans "Meshory on your home server" with Docker).

**Adopted into the milestones above:**

| From Meshory | Where |
|---|---|
| RAR (with split sets), 7Z, TAR/TGZ archives | M3 |
| Readmes, SVGs and PDFs in the app; video files; images sized for print (DPI, printed size) | M3 |
| G-code embedded thumbnails and layer preview, FreeCAD files, LightBurn previews | M3 |
| Folder duplicates, "one pack inside another", archive rollups, source names and modified time in duplicates, faster bulk cleanup | M4 |
| Exclude folders, include patterns, folders with no models, pause/resume scans, scan issues with retry, offline sources, rescan one folder, rescan from scratch | M5 |
| Pinned tags, sharper tag search, nested and smart collections, where a model came from (source, designer, licence), custom thumbnails, folder covers, print queue boards | M6 |
| Model sets (a multi-part figure as one card, suggested from folders or built by hand) | M7 |
| Your own AI tagging rules, AI runs you can stop and watch, MCP server (community vote) | M8 |
| Viewer measuring tools, hover quick look, studio thumbnails, light theme, inches, help beside settings, type to search, custom slicers, scheduled backups | M9 |
| Move files between folders (sort a downloads folder; the library follows without a rescan), rename, trash, hide single files, trash from the preview | M11, opt-in, because MeshManager is read-only by default |
| Filament inventory (spools, printable with what's on hand) | M10 |
| Import from Patreon, Printables & MakerWorld (browser extension) | M12 |
| STEP, FBX, Rhino, Fusion, LYS, HEIC, PDF/video thumbnails, DXF/DWG/xTool, 3MF converter, versioning, nested-archive search | M13 (heavy native tools deferred, to keep one small image) |

**Not applicable to a self-hosted web app:** global shortcut, window title bar, update prompts and automatic updates (Docker images instead), licence seats, Finder tags on Mac, OneDrive placeholder files, database recovery window (Postgres backups instead), drag-out to desktop apps (downloads instead), "one library on several computers" (a server is already shared by every browser).
