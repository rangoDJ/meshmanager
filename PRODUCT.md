# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- **The owner**, a 3D-printing hobbyist with a large, messy hoard of model files (downloads, Patreon packs, ZIPs) on a home server. Uses MeshManager from a desktop browser for browsing and sorting, and from a phone or tablet near the printer or on the go.
- **Household members**, who browse the library and pick things to print. They are less technical than the owner.

## Product Purpose

A self-hosted library for 3D print files. It indexes the folders where models already live, shows every model as a thumbnail, opens models and their companion files (readmes, PDFs, photos, sliced G-code) in the browser, and helps find duplicates and organize. Success: the owner finds and prints models they already own instead of re-downloading them, and the downloads pile becomes a library.

The jobs people open it for:

1. Find something to print: browse or search the hoard and pick the next print.
2. Sort a messy downloads pile: make sense of packs, archives and duplicates.
3. Check before slicing: inspect a model, its readme/PDF, or a sliced G-code before it goes to the printer.
4. Track what was printed: print queue, what printed or failed, filament use.

## Positioning

The self-hosted, browser-based counterpart to Meshory (a paid desktop app, used as the feature reference). One small Docker container next to Postgres on the owner's own server; every device on the network opens the same library in a browser, with no install and no cloud. It never moves, renames or uploads files: libraries are mounted read-only.

## Operating Context

- Runs on the owner's home server (Docker, managed with Dockhand), LAN only, behind no public proxy by choice.
- Library: model packs from the owner's downloads folder, including ZIP/RAR/7Z archives as distributed by designers.
- Printing: Bambu Lab printers sliced in Bambu Studio, so 3MF projects with plates and filament colours, and Bambu-sliced G-code, are the most common formats.
- Used on desktop and on phones/tablets, often next to the printer.

## Capabilities and Constraints

- Read-only access to libraries; file management (move, rename, trash) is planned as an opt-in per source.
- App settings live in the UI and `DATA_DIR/config.json`; environment variables are for deployment only. Deployments stay minimal: one app container plus Postgres, no sidecars unless a feature truly needs one.
- Pure-Go backend; formats that need heavy native tools (STEP, FBX, HEIC, PDF/video thumbnails) are deferred.
- Single admin account today; household members point to multiple users and roles later (planned in PLAN.md).
- Roadmap and milestone order: `PLAN.md` §6.

## Brand Commitments

- Name: MeshManager. Icon: the cube mark in `frontend/public/favicon.svg`.
- No other binding brand assets, voice guide or references have been set.

## Evidence on Hand

None. There are no testimonials, users beyond the owner's household, benchmarks or press; do not invent any.

## Product Principles

1. **The owner's files are sacred.** Never write to the library without an explicit, opt-in choice; show where files really are.
2. **Find what you already own.** Every model, including the ones buried in archives and forgotten folders, is visible and searchable.
3. **Check before you print.** Everything needed to decide (3D view, plates, readme, sliced toolpaths, size and weight) is one click from the file.
4. **Simple to run.** One container, settings in the UI, sensible defaults.
5. **Works for the household.** Plain language and touch-friendly controls, because not everyone using it is the owner, and not every session is at a desk.

## Accessibility & Inclusion

No formal standard has been set. Known needs: comfortable touch targets and layouts on phones and tablets, and plain-language labels for less technical household members.
