# Desktop Tray Popover Contract

The desktop tray surface is a watched `DesktopTrayState`. The native tray menu
and any rich popover render the same ordered `DesktopTrayEntry` tree from
`bldr/desktop/tray/tray.proto`.

The rich popover is the current opt-in rich panel implementation. It renders
this contract, may use Electron-owned `DesktopRuntimeState` only for
presentation counts, and does not own separate runtime state, invent separate
actions, or bypass the tray resource. The native menu remains a complete
fallback renderer for the same tree.

## State Owner

`DesktopTrayResourceService.WatchDesktopTray` streams the full tray tree. Each
snapshot contains:

- `entries`: ordered tray rows.
- `icon_state`: collapsed tray icon status.
- `status_text`: short tray tooltip text.

Entries are registered through `RegisterDesktopTrayEntry` and updated through
the returned `DesktopTrayEntryResourceService` resource. Releasing that resource
removes the entry. Callers that need custom action handling attach a
`DesktopTrayActionHandlerService` resource and publish an action with
`DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER`.

The desktop runtime status projector is one producer of this tree. It projects
runtime health, listener state, sessions, Spaces, activity, updates, and app
commands into tray entries.

Electron main already owns the current `DesktopRuntimeState` snapshot for
desktop-shell lifecycle. Rich panel descriptors can read that snapshot to enrich
headers, counts, and cards, but command identity, visibility, ordering,
enablement, and invocation semantics still come from `DesktopTrayState.entries`.

## Tree Shape

Each `DesktopTrayEntry` has a stable `id`. IDs are unique within the tray tree
and are the invocation key for attached handlers.

The registry sorts entries by:

1. `path`, joined by path segment.
2. `group`.
3. `order`.
4. `id`.
5. resource id.

The status projector assigns `order` from the projected list index, so its
published sections render in projection order.

`path` places a row below nested containers. In the native menu renderer each
non-empty path segment creates or reuses a submenu level. An entry with kind
`DESKTOP_TRAY_ENTRY_KIND_SUBMENU` reserves a submenu container from its label;
it does not render a selectable row.

## Entry Kinds

`DESKTOP_TRAY_ENTRY_KIND_SECTION` is a disabled section heading.

`DESKTOP_TRAY_ENTRY_KIND_SEPARATOR` is a native separator.

`DESKTOP_TRAY_ENTRY_KIND_STATUS` is a non-invokable status row. A rich popover
may use `detail`, `status_text`, `icon_name`, `icon_state`, `severity`, and
`active` to add visual hierarchy. The native menu fallback renders it as a
disabled label.

`DESKTOP_TRAY_ENTRY_KIND_ACTION` is an invokable row when `enabled` is true and
`action` is set. Disabled actions remain visible but cannot run.

`DESKTOP_TRAY_ENTRY_KIND_UNSPECIFIED` has no row semantics and renders like a
disabled label in the native fallback.

## Actions

Action rows use `DesktopTrayAction.kind`:

- `DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE`: open or focus the singleton app window
  at `route`; an empty route focuses the window.
- `DESKTOP_TRAY_ACTION_KIND_NEW_WINDOW`: open `route` in a new app window; an
  empty route opens the app root.
- `DESKTOP_TRAY_ACTION_KIND_COPY_TEXT`: copy `value` to the native clipboard.
- `DESKTOP_TRAY_ACTION_KIND_REVEAL_PATH`: reveal filesystem path `value`.
- `DESKTOP_TRAY_ACTION_KIND_QUIT`: request explicit desktop runtime quit.
- `DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER`: call
  `InvokeDesktopTrayEntry(entry.id)` on the tray resource.

A rich popover routes these actions through the same command paths as the native
menu. It does not invoke attached handler resources directly.

## Runtime Projection

The current desktop runtime projection publishes a compact native-safe tree:

- Title status row: `Spacewave: <status>`.
- Open commands: `Open Spacewave`, `New Window`.
- Status section: runtime and update status.
- Sessions section: route-backed session rows, or `No sessions`.
- Spaces section: route-backed Space rows, or `No spaces`.
- Activity section when activity exists.
- Quick Actions section for update install, synthetic diagnostics actions, and
  runtime actions.
- App commands: settings, about, quit.

When attention items exist, the projection switches to an attention-focused
tree: title, primary attention status, optional attention detail, open action,
ready update action, and quit.

The projected title row drives `DesktopTrayState.status_text` by trimming the
`Spacewave: ` prefix. The tray registry derives `icon_state` as the highest
entry icon state in the snapshot.

## Popover Rendering Boundary

A rich popover may provide denser visual layout than the native menu, but it
renders the `DesktopTrayState` contract:

- It preserves entry ordering and submenu/path grouping.
- It shows all native-visible entries unless a row is represented by an
  equivalent richer section header or separator.
- It treats status, section, separator, submenu, and disabled action rows as
  non-invokable.
- It exposes action rows only when `enabled` is true and `action` is present.
- It uses `severity`, `icon_state`, and `active` as presentation hints, not as
  alternate action semantics.
- It updates from the watch stream; it does not poll cloud or runtime state from
  the frontend to render tray contents.

The native menu remains valid and complete when the popover is disabled, hidden,
or fails to attach. Electron main always rebuilds the native context menu from
`WatchDesktopTray`; the popover is optional UI layered on top of that resource.

The current rich panel remains opt-in until screenshot and interaction proof
accepts macOS enablement:

```bash
BLDR_ELECTRON_DESKTOP_TRAY_POPOVER=1 bun run start:desktop
```

Renderer packaging decision as of 2026-05-30: the opt-in panel stays as inline
data-URL HTML owned by Electron main. That keeps the current surface at the
descriptor-view boundary: Electron main adapts `DesktopTrayState` plus its
existing `DesktopRuntimeState` snapshot into static panel HTML/CSS/JS, and the
panel sends only action URLs back to the shared dispatcher. It does not import
the app entrypoint, open a `WebRuntime` connection, subscribe to
`WatchDesktopState`, poll cloud state, or read filesystem state.

A future bundled panel surface is a promotion decision, not a prerequisite for
this opt-in layer. If the panel moves into a bundled renderer, the bundle must
keep the same contract: one descriptor input from Electron main, action events
back to Electron main, and no independent runtime/resource polling inside the
panel.

Dynamic macOS icon rendering, quiet native notifications, and a global tray
toggle shortcut are also opt-in. `BLDR_ELECTRON_DESKTOP_TRAY_DYNAMIC_ICON=1`
enables generated template icon variants,
`BLDR_ELECTRON_DESKTOP_TRAY_NOTIFICATIONS=1` enables quiet update-ready and
critical-attention notifications, and
`BLDR_ELECTRON_DESKTOP_TRAY_TOGGLE_SHORTCUT=<accelerator>` registers a
process-owned toggle shortcut. All three default off and are cleaned up by
Electron main on quit.

## Native Menu Fallback Boundary

The fallback renderer is intentionally lower fidelity:

- It supports labels, disabled rows, separators, submenus, and click handlers.
- It ignores visual-only hints such as `detail`, `status_text`, `icon_name`,
  `severity`, and `active` unless they are already folded into `label`.
- It disables copy and reveal actions when `value` is empty.
- It disables unknown action kinds.
- It opens or focuses the main window on tray click when no popover handles the
  click.

Because the native renderer is complete, producers must keep labels concise and
self-contained. Popover-only metadata can enrich the display, but the label
continues to carry the fallback meaning.
