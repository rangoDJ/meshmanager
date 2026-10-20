---
name: MeshManager
description: A self-hosted library for 3D print files, opened from any browser on the home network.
colors:
  indigo-accent: "#4f46e5"
  indigo-accent-dark: "#818cf8"
  accent-ink: "#ffffff"
  accent-ink-dark: "#0c0c0f"
  cool-paper: "#f6f6f8"
  cool-paper-dark: "#0c0c0f"
  sheet: "#ffffff"
  sheet-dark: "#16161b"
  well: "#f0f0f3"
  well-dark: "#1f1f26"
  hairline: "#e3e3e8"
  hairline-dark: "#2b2b33"
  ink: "#18181b"
  ink-dark: "#f2f2f5"
  pencil: "#6b6b76"
  pencil-dark: "#a0a0ab"
  danger-red: "#dc2626"
  danger-red-dark: "#f87171"
  success-green: "#15803d"
  success-green-dark: "#4ade80"
  warning-amber: "#b45309"
  warning-amber-dark: "#fbbf24"
  tag-slate: "#64748b"
  tag-slate-dark: "#94a3b8"
  tag-red: "#dc2626"
  tag-red-dark: "#f87171"
  tag-orange: "#ea580c"
  tag-orange-dark: "#fb923c"
  tag-amber: "#d97706"
  tag-amber-dark: "#fbbf24"
  tag-green: "#16a34a"
  tag-green-dark: "#4ade80"
  tag-teal: "#0d9488"
  tag-teal-dark: "#2dd4bf"
  tag-blue: "#2563eb"
  tag-blue-dark: "#60a5fa"
  tag-indigo: "#4f46e5"
  tag-indigo-dark: "#818cf8"
  tag-violet: "#7c3aed"
  tag-violet-dark: "#a78bfa"
  tag-pink: "#db2777"
  tag-pink-dark: "#f472b6"
typography:
  headline:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: "2rem"
  title:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: "1.5rem"
  body:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: "1.25rem"
  label:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: "1rem"
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.625
rounded:
  md: "6px"
  lg: "8px"
  xl: "12px"
  2xl: "16px"
  full: "9999px"
spacing:
  "1.5": "6px"
  "2": "8px"
  "3": "12px"
  "4": "16px"
  "5": "20px"
  "6": "24px"
  "8": "32px"
components:
  button-primary:
    backgroundColor: "{colors.indigo-accent}"
    textColor: "{colors.accent-ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "8px 14px"
  button-secondary:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "8px 14px"
  button-secondary-hover:
    backgroundColor: "{colors.well}"
  button-ghost:
    textColor: "{colors.pencil}"
    rounded: "{rounded.lg}"
    padding: "8px 14px"
  button-ghost-hover:
    backgroundColor: "{colors.well}"
    textColor: "{colors.ink}"
  input:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "8px 12px"
  card:
    backgroundColor: "{colors.sheet}"
    rounded: "{rounded.xl}"
    padding: "20px"
  nav-item:
    textColor: "{colors.pencil}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "8px 12px"
  nav-item-active:
    textColor: "{colors.indigo-accent}"
  tag-chip:
    backgroundColor: "{colors.well}"
    textColor: "{colors.ink}"
    typography: "{typography.label}"
    rounded: "{rounded.full}"
    padding: "2px 4px 2px 10px"
  tag-dot:
    backgroundColor: "{colors.tag-blue}"
    rounded: "{rounded.full}"
    size: "10px"
  tag-swatch:
    backgroundColor: "{colors.tag-blue}"
    rounded: "{rounded.full}"
    size: "16px"
  button-danger:
    backgroundColor: "{colors.danger-red}"
    textColor: "{colors.accent-ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "8px 14px"
  selection-bar:
    backgroundColor: "{colors.sheet}"
    rounded: "{rounded.xl}"
    height: "40px"
  viewer-chrome:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.pencil}"
    rounded: "{rounded.lg}"
  tool-button-active:
    backgroundColor: "{colors.indigo-accent}"
    textColor: "{colors.accent-ink}"
    padding: "6px 10px"
  touch-step-button:
    textColor: "{colors.pencil}"
    rounded: "{rounded.lg}"
    size: "40px"
---

# Design System: MeshManager

## Overview

**Creative North Star: "The Quiet Workbench"**

MeshManager is a tool laid out on a clean bench: neutral cool-grey surfaces, hairline borders, one indigo accent, and the file itself (a model, a toolpath, a photo, a readme) as the only thing with colour or weight. Organizing the library (tags, collections, a print queue) follows the same rule: the controls are neutral, and the user's own labels are the only colour they add, as small dots. The chrome stays small, grey and out of the way until it is touched; the content area gets the room.

The system is flat and tonal. Depth comes from three neutral steps (page, sheet, well) and 1px hairlines, not shadows. It follows the operating system's light or dark preference, and every surface token has a matched dark value, so both modes are first-class. Density is moderate: 14px body text, compact controls on desktop, and 40px step targets plus sideways-scrolling control strips on phones, because the library is used next to the printer as often as at a desk.

Viewers share one frame: a full-screen content area with floating translucent controls in the corners, and an information panel that sits on the right on wide screens and drops below the content as a bottom sheet on phones.

**Key Characteristics:**
- One accent (indigo) for action, selection and progress; everything else is neutral.
- Flat surfaces separated by hairline borders and tonal steps.
- Floating viewer controls on translucent, blurred sheets.
- System UI typeface throughout; tabular figures for every measurement.
- Light and dark follow the OS, token for token.
- User-chosen tag colours appear only as small dots on neutral chips, never as tinted chips, text, borders or fills.

## Colors

A cool neutral ramp with a single indigo voice, plus three status colours used only to report state.

### Primary
- **Workshop Indigo** (indigo-accent; indigo-accent-dark in dark mode): primary buttons, the active view preset and tool toggle, the active nav item (as text on a 10% indigo wash), progress bars, range sliders (`accent-color`), focus rings and outlines, the text caret, links, and text selection (35% mix).
- **Accent Ink** (accent-ink / accent-ink-dark): text and icons sitting on filled indigo.

### Neutral
- **Cool Paper** (cool-paper): the page background and the full-screen viewer backdrop.
- **Sheet** (sheet): cards, sidebar, info panel, inputs, dialogs, and the translucent viewer controls (at 90% opacity).
- **Well** (well): hover fills, skeleton loaders, progress tracks, code blocks, the media stage tint (at 40%).
- **Hairline** (hairline): every border and divider, and the scrollbar thumb.
- **Ink** (ink): primary text.
- **Pencil** (pencil): secondary text, labels in definition lists, idle icons and controls.

### Status
- **Danger Red**, **Success Green**, **Warning Amber**: errors (as text on a 10% tint), confirmations such as the copied check mark, and warnings. Never decorative.

### Tag Colours
Ten user-assignable label colours, each with a matched dark value (--tag-* in the stylesheet): slate, red, orange, amber, green, teal, blue, indigo, violet, pink. A tag's colour is a label the user chose, so it is content. It is shown only as a dot: 10px on chips, tiles, filter rows and suggestions, 16px as swatches in the colour picker and on the Tags page. The tag name, its chip, its border and its row stay neutral. Tag indigo equals the interface accent in value but is a label here, not an action.

### Named Rules
**The One Voice Rule.** Indigo is the only chromatic colour in the interface chrome. If a control is not acting, selected, focused or in progress, it is neutral.

**The Content Owns Colour Rule.** Saturated colour beyond indigo appears only inside the content: the model, the photo, the G-code toolpath with its line-type legend, and the colours users assign to their own labels (tags), which show as small dots only.

**The Dot-Only Rule.** A tag colour is a 10px dot (16px as a swatch in a picker), never a tinted chip, coloured text, border or fill. A tag chip is always neutral (well fill, hairline border) with the dot inside it.

## Typography

**Body Font:** the system UI stack (ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif)
**Mono Font:** Tailwind's ui-monospace stack, for file paths and plain-text files

**Character:** a utility face that looks native on every device in the house; current hierarchy comes from size and weight steps. No display role is defined yet; page titles are headline-sized UI text.

### Hierarchy
- **Headline** (600, 1.5rem): page titles (Dashboard, Sources, Settings) and Markdown h1.
- **Title** (600, 1rem): card headings, the file name at the top of a viewer's info panel.
- **Body** (400 or 500, 0.875rem): almost all UI text, buttons, nav, form fields, info panel content. Weight 500 marks labels and button text.
- **Label** (400, 0.75rem): metadata lines, hints, the "N of M" counter, segmented-control labels.
- **Mono** (400, 13px, relaxed leading): host paths in Copy Location and TXT files. Markdown and TXT reading columns are capped at 72ch.

### Named Rules
**The Tabular Measure Rule.** Every number that is a measurement or a count (sizes, dpi, layers, grams, durations, positions) uses tabular figures.

**The Format Code Rule.** File format codes are shown uppercase (STL, 3MF, JPG) as data; uppercase is not used for headings or section labels.

## Layout

A fixed 240px sidebar (drawer below 768px, behind a 40% black scrim) and a content column. The library, collections and print queue run full width; other pages (including Tags) are capped at 64rem (max-w-5xl). Page padding is 16px on phones, 24px from md, 32px on narrow-content pages. Spacing follows Tailwind's 4px grid, mostly 8px gaps between controls, 12-16px inside panels, 20px card padding.

Viewers are full-screen and split content and info: a column on phones (info panel below, capped at 42vh, scrolling) and a row from lg (1024px) with a 320px info panel on the right. Floating controls sit 12px from the content edges. Content leaves a 64px top inset so it clears the controls; the embedded PDF sits below a 56px strip so viewer controls never cover the browser's PDF toolbar. On coarse pointers, PDFs are handed to the browser's own reader through an "Open PDF" panel instead of an iframe. The view-preset strip scrolls sideways (scrollbar hidden) when the screen is too narrow for it.

The library has a left panel beside the grid: the folder tree, then a Tags section (click a tag to require it, click again to rule it out, a third time to clear; an All / Any switch appears once two or more are required; "Not in a collection" sits below). On phones the panel is a drawer with a header and Close button and a primary "Show N files" footer. Above the grid, folder cards (4.5rem tall, 2 across on phones, auto-filling 14rem columns from sm) show a cover thumbnail, name and file count. Collections uses a nav tree on lg and a select on phones; Print queue uses a 16rem list column grouped by printer on lg and a select on phones. The print board is a container query: three columns (To print, Printing, Done) from the @2xl container width, stacked below it with Printing first. An `@media print` block forces a light palette so a printed list is black on white.

**The Thumb Reach Rule.** On phones, repeated stepping controls (layer up/down, play) are 40px square and sit in a bottom bar; bulk-bar buttons, print-card advance buttons and combobox suggestions are 40px tall; the desktop equivalent is a vertical rail with compact 28px buttons.

## Elevation & Depth

Flat by default. Depth is carried by the three neutral steps (cool-paper, sheet, well) and hairline borders. Shadows are reserved for surfaces that leave the page: dialogs and the file drawer (a large soft shadow), the sign-in card (a faint one), and the SVG paper sheet in the file viewer, which needs to read as a physical page on any backdrop. Floating viewer controls, the bulk selection bar and the pinned-tag pill on a tile use translucency and backdrop blur instead of shadow.

### Shadow Vocabulary
- **Overlay** (`box-shadow: 0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1)`): dialogs and the side drawer.
- **Resting hint** (`box-shadow: 0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)`): the sign-in card only.
- **Paper** (`box-shadow: 0 2px 12px rgb(0 0 0 / 0.25)`): white paper behind SVG cut files.

### Named Rules
**The Flat Page Rule.** Cards, panels and list rows never carry a shadow; if something needs separating, use a hairline or a tonal step.

**The Frosted Chrome Rule.** Controls that float over content (viewer controls, the selection bar, the pinned-tag pill) are sheet at 90% opacity with backdrop blur and a hairline border.

## Shapes

Softly rounded rectangles throughout. Controls, buttons, inputs, control groups and media are 8px; small inline controls (selects in panels, the copy button, rail buttons) are 6px; cards and larger floating panels (layer rail, phone layer bar, drawer tiles) are 12px; dialogs are 16px, and on phones they become a bottom sheet with only the top corners rounded. Pills (full radius) are for progress bars, avatars and line-type swatches. Grouped controls share one rounded container with internal dividers rather than separate rounded buttons.

## Components

### Buttons
Quiet and compact; the primary one is the only filled thing on a screen.
- **Shape:** gently rounded (8px).
- **Primary:** indigo fill, accent-ink text, 500 weight, 8px by 14px padding; hover lowers opacity to 90%.
- **Secondary:** sheet fill with hairline border; hover fills with well.
- **Ghost:** no fill, pencil text; hover fills with well and turns text to ink.
- **Destructive:** danger-red fill with accent-ink text, used in confirm dialogs (delete a list, collection or tag), never as a resting button.
- **Disabled / loading:** 50% opacity, not-allowed cursor; loading adds a spinning ring before the label.

### Inputs / Fields
- **Style:** sheet background, hairline border, 8px radius, 8px by 12px padding, 14px text, label above in 500 weight, hint below in 12px pencil.
- **Focus:** border turns indigo with a 2px indigo ring at 25%.

### Cards / Containers
- **Corner Style:** 12px.
- **Background:** sheet, hairline border, no shadow.
- **Internal Padding:** 20px, with a title row (title left, action right) 16px above the content.
- **Empty states:** dashed hairline border, centred, generous vertical padding.
- **Inline editing:** a source card swaps into its edit form in place (deep-linked with `?edit=<id>`), rather than opening a dialog.

### Navigation
- **Sidebar:** 240px sheet with a 56px brand row (cube mark plus name). Items are 14px with a 16px icon, pencil by default, well fill on hover, indigo text on a 10% indigo wash when active.
- **Mobile:** a 56px top bar with a menu button; the sidebar slides in over a scrim.

### Tag Chips and Dots
Neutral labels; the colour is a dot and nothing else.
- **Chip:** full-radius pill, well fill, hairline border, 12px text, a 10px dot at the left; with a remove button (24px, 36px on coarse pointers) when editable. The name truncates.
- **Combobox:** a normal text field with suggestions listed in the page flow beneath it (never a floating popover), each row 40px with dot and name, plus "Create tag" for a new name. Arrow keys, Enter and Escape work.
- **Colour picker:** a radio group of 36px square swatch buttons, each holding a 16px dot; the chosen one gets an indigo border on a 10% wash.
- **Tile:** up to five pinned-tag dots in a frosted full-radius pill at the tile's bottom-left.

### Library Selection
- **Select mode:** tiles gain a 24px checkbox at top-left; a chosen tile gets an indigo border and ring; shift-click selects a range.
- **Bulk bar:** fixed at the bottom, at most 672px wide, frosted chrome at 12px radius. Tag, Collection and Print queue buttons (40px tall, icon and word always visible) open an inline panel above the bar; the open one is indigo text on a 10% wash. Confirmation text appears in success green for four seconds.
- **Folder cards:** 12px radius, hairline border, 44-56px cover thumbnail, name and tabular file count; the border goes to 60% indigo on hover.

### Print Queue Board
Three flat columns of cards, each 12px radius with a hairline border and 12px padding: 44px thumbnail, two-line name, uppercase format and size line, a "more" button, a note field (well fill that turns sheet with an indigo ring on focus), a secondary advance button with an indigo icon (Start printing, Mark done, Print again), and a quantity stepper (tabular figures, 32px buttons, 40px on coarse pointers). The "more" panel opens inline under a hairline with Back to To print, Move up / down and Take off the list. Empty columns show a dashed hairline note. The page toolbar holds 40px Edit, CSV, Print or PDF, Clear done and Delete buttons. A print-only table sheet replaces the board on paper.

### Viewer Chrome (signature)
The shared frame for every file viewer.
- **Corner:** top-right, a grouped previous / "N of M" / next control and a separate close button, both frosted chrome. Arrow keys step, Escape closes, horizontal swipe steps on touch.
- **Tool strip:** top-left, a segmented control of view presets plus grouped icon toggles. Toggles are pencil when idle and indigo-filled when pressed; focus shows an inset 2px indigo outline.
- **Info Panel:** title-weight file name, an uppercase format and size line, then definition lists (pencil terms, tabular values), Copy Location, and paired Download / New tab actions.
- **Loading:** a quiet 192px progress line in the content area with a 12px caption, never a full-screen spinner.
- **Problems:** a centred muted icon, a 500-weight sentence and a plain-language next step ("Download it to open it in another app").

### File Viewer
- **Images:** fit to the stage; click toggles actual size. The info panel adds a print-size block: a bordered 8px box with a DPI select and the result in 16px 500-weight tabular figures.
- **SVG:** shown on white paper with the paper shadow, since cut files are usually dark lines on nothing.
- **Video:** native controls on a black, 8px-rounded frame.
- **Markdown / TXT:** a 72ch reading column with skeleton lines while loading.

### G-code Layer Preview
- **Rail:** on lg, a 56px-wide vertical frosted rail on the right holding up / slider / down / play; on phones, a bottom frosted bar with 40px step buttons around a horizontal slider and a centred layer caption.
- **Scene:** lower layers are mixed 86% toward the chosen backdrop so the current layer stands out on any background. Viewer backdrops: Dark #16161b, Graphite #2b2d33, Studio grey #8a8f99, Light #f2f2f5.
- **Line types:** slicer-style feature colours, shown in the canvas and in the info panel legend as 16px pill swatches: Outer wall #f59e0b, Inner wall #fcd34d, Infill #ef4444, Solid infill #a855f7, Top/bottom #ec4899, Bridge #3b82f6, Support #22c55e, Skirt/brim #14b8a6, Overhang #0ea5e9, Other #9aa0ab; travel moves #7c8cff at 55-60%.

### Copy Location
The host path in 13px mono that breaks anywhere, with a small 6px-rounded bordered "Copy" button that swaps to a green check when copied. When no network location is set, it says so in plain words and links to the source's edit form.

## Do's and Don'ts

### Do:
- **Do** take every colour from the theme tokens so light and dark modes stay matched; the only hard-coded colours belong to the 3D/toolpath canvas, its backdrops and its legend.
- **Do** float controls over content as frosted chrome: sheet at 90%, backdrop blur, hairline border, 8px radius.
- **Do** give phone-only stepping controls 40px square targets, and let overflowing control strips scroll sideways instead of wrapping into a second row of tiny buttons.
- **Do** use tabular figures for measurements and counts, and write units in full plain words where space allows ("42 min", "8.6 g").
- **Do** show a tag's colour only as a dot (10px; 16px in pickers) on a neutral chip, row or tile.
- **Do** keep every bulk and queue control's label visible with a 40px target, and open its panel inline above the bar rather than in a popover.
- **Do** move the info panel below the content on phones and to a 320px right column from 1024px.

### Don't:
- **Don't** put shadows on cards, panels or list rows; shadows are for dialogs, the drawer and SVG paper.
- **Don't** introduce a second interface accent; feature colours stay inside the toolpath canvas and its legend, and tag colours stay dots.
- **Don't** tint a tag chip, its text, border, row or fill with the tag's colour.
- **Don't** cover a browser's own PDF toolbar with viewer controls, and don't embed PDFs on coarse pointers.
