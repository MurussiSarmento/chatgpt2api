# AGENTS.md

## Frontend (`web/`)

- A build-free HTML + CSS + JS trio: daisyUI 5 provides component styles, Alpine.js handles interactions. `index.html` holds the shell and each page template `<template id="page-<name>">`, `index.css` holds tokens and styles, `index.js` holds requests, shared functions, Web Components, and page components.
- Adding a page: add a `<template id="page-<name>">` whose root node is `x-data="<name>"`; register it in `index.js` with `Alpine.data("<name>", ...)`, and register it in `routes` and the navigation; release timers and charts in `destroy()`.
- Styling uses only the semantic tokens at the top of `index.css` — never write raw color values or raw pixels:
  - Font sizes: body 13px, secondary 12px, section title 14px, page title 18px, metric numbers 24px.
  - Spacing: multiples of 4 (`--space-1…12`), 4–8 inside controls, 12–16 inside components, 24 between sections, 32 page margins.
  - Control heights 24 / 32 / 40, admin UI defaults to `sm`.
  - Border radius uses `--radius-control` / `--radius-panel` / `--radius-dialog`; a child's radius = parent radius − padding.
  - Cards use the `--color-surface-muted` gray background with no border; body areas such as tables, forms, and sidebars keep a white background, and tables sit directly on the page separated by row lines; shadows are only for dropdowns and popups.
  - Inputs, dropdowns, and checkboxes use a light border, and focus only darkens the border; the selected state uses a light-gray border.
- Layout: content pages have a `.page` root node with `.page-head` as the first row; full-height workspaces use `.workspace`; arrange with `.vstack`, `.row`, `.grid`; cards use `.panel`, body areas use `.panel.panel-flush`. Ordinary pages keep only a single layer of vertical scrolling, and tables and lists are not height-capped.
- Settings page: a `.settings-nav` + `.nav-item` grouping on the left, and on the right each row is a `.setting-row` (name plus a one-line description on the left, control on the right), with long controls using `.setting-row-block`; place "Undo" and "Save" in the top-right corner, disabled when there are no changes, show a dot on groups with unsaved changes, and save only submits the groups that changed.
- Modals default to 560px wide, `modal-md` is 720px, `modal-wide` is 880px, with the height scrolling internally; when there are many details, place a segmented control in `.modal-head` to split into tabs, put the close button on the right of the title row, and use `.inline-stats` for key status.
- Component choices:
  - Primary action is `btn btn-sm btn-primary`, only one per area; ordinary is `btn btn-sm`; secondary and inline are `btn btn-ghost btn-sm` (icon-only adds `btn-square`, `aria-label`, `tooltip`); delete is `btn btn-sm btn-error btn-soft` + `confirmDialog()`.
  - Switching views of the same data uses `tabs tabs-box tabs-sm`; switching business sections uses `tabs tabs-border`.
  - A toggle that takes effect immediately uses `toggle toggle-sm`; boolean items saved with a form: the settings page uses an "On / Off" segmented control, other forms use `checkbox checkbox-sm` + `.check-row`.
  - Options use `<ui-select>` (add `multiple` for multi-select, value is an array), dates use `<ui-date>`, pagination uses `<ui-pager>`, and read-only status uses `badge badge-sm badge-soft`.
  - Field descriptions use an `info` icon after the label + `tooltip`; page-level issues use `alert alert-soft alert-warning`; failure notices use `toast(text, type)`; empty states use `.empty` + an icon + a one-line message; loading uses `skeleton` or `loading loading-spinner loading-xs`.
  - Copyable fields use `.copyable` + `$copy`.
- Copy: write only the title, field names, and key values, and tuck descriptions into tooltips; use brief, written business language; buttons state the action, table headers state the business name; show the real reason on failure.
- Scripts: requests are unified under `api(path, { method, body })` (binary `api.blob`, streaming `api.stream`), and pages write the path directly; template magic methods are `$time`, `$short`, `$num`, `$bytes`, `$duration`, `$md`, `$copy`, `$go`; global functions are `toast`, `confirmDialog`, `lightbox`, `copyText`, `markdown`, `saveBlob`, `readDataUrl`, `urlToFile`, `sleep`; icons use `<ui-icon name="...">` (see the icon set at the top of `index.html`).
- Avoid custom class names already taken by daisyUI (`stack`, `stat`, `chat`, `prose`); use `vstack`, `row`, `grid` for layout, `metric-*` for metric cards, and `markdown` for Markdown body text.
