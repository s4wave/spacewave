---
title: Desktop Daemon Console
section: internals
order: 9
summary: DesktopTrayEntry publication, background runtime behavior, and desktop status debugging.
---

## Overview

Spacewave desktop keeps a native status surface alive for the process lifetime. On macOS this is the menu bar extra. On Windows it is the notification-area icon. On Linux it is the desktop environment's tray/status item when the environment exposes one.

The tray is a daemon console, not a second app shell. It renders the published `DesktopTrayEntry` tree, opens or focuses the singleton Spacewave window, exposes safe diagnostics, and provides explicit quit. Closing every Electron window does not stop the background runtime when the project uses tray-backed desktop presence.

## Resource Trees

Electron main owns the generic desktop runtime Resource SDK tree in `bldr/web/electron/main/desktop-runtime.ts`. The root service is `DesktopRuntimeResourceService`:

| RPC                     | Purpose                                                                                         |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| `WatchDesktopState`     | Streams the latest `DesktopRuntimeState` to popover/debug clients.                              |
| `SetDesktopState`       | Publishes projected status from the runtime side into Electron main.                            |
| `OpenOrFocusMainWindow` | Opens or focuses the singleton app window, optionally at a route.                               |
| `QuitDesktopRuntime`    | Marks the runtime as quitting and requests an explicit shutdown.                                |

Electron main also exposes a resource-backed `DesktopTrayResourceService`. Runtime publishers register `DesktopTrayEntry` rows there, and native tray renderers subscribe to `WatchDesktopTray` for the ordered menu tree. This `DesktopTrayEntry` publication is the active native tray model.

`DesktopTrayState` contains the native tray contract:

| Field        | Owner                 | Meaning                                                                       |
| ------------ | --------------------- | ----------------------------------------------------------------------------- |
| `entries`    | Runtime publishers    | Ordered `DesktopTrayEntry` rows rendered into the native menu tree.           |
| `iconState`  | `DesktopTray` service | Collapsed tray icon state derived from active published rows.                 |
| `statusText` | `DesktopTray` service | Collapsed tray status text derived from the published title/status row.       |

Each `DesktopTrayEntry` carries stable row identity, ordering, section/path placement, display label, active/enabled state, severity/icon hints, and optional action transport. Route, new-window, copy, reveal, quit, and attached-handler actions are represented on the entry itself, so native menu dispatch does not depend on `DesktopRuntimeState`.

`DesktopRuntimeState` is the Spacewave status projection input and popover/debug presentation state:

| Field                               | Owner         | Meaning                                                                                    |
| ----------------------------------- | ------------- | ------------------------------------------------------------------------------------------ |
| `mainWindowOpen`                    | Electron main | Whether the singleton app window currently exists.                                         |
| `quitting`                          | Electron main | Whether explicit quit has started.                                                         |
| `statusText`, `health`, `lifecycle` | Projector     | Collapsed icon/menu status such as running, syncing, attention, disconnected, or quitting. |
| `listener`                          | Projector     | CLI listener reachability, socket path, and connected-client count.                        |
| `sessions`                          | Projector     | Bounded session rows with labels, provider/account hints, routes, and status text.         |
| `spaces`                            | Projector     | Bounded recent/open Space rows with full-app handoff routes.                               |
| `activity`                          | Projector     | Bounded recent sync/runtime activity rows.                                                 |
| `update`                            | Projector     | Native update readiness when the desktop app must act.                                     |
| `attentionItems`                    | Projector     | User-actionable attention rows using the desktop runtime attention taxonomy.               |
| `actions`                           | Projector     | Explicit safe commands such as open route, copy text, reveal path, or quit.                |

Electron main preserves `mainWindowOpen` and `quitting` when `SetDesktopState` publishes runtime projection. This prevents background status publishers from accidentally reopening or un-quitting the app.

The root resource is registered as an Electron main Resource SDK server
extension. Tray consumers, projectors, and command handlers all enter through
that resource tree instead of reaching into BrowserWindow state. This keeps the
desktop daemon console process-lifetime scoped: renderer windows can come and
go while `WatchDesktopState`, tray entries, and CLI listener status remain
available in Electron main.

## Projection Flow

Spacewave-specific interpretation lives in `core/resource/desktop/statusprojector`. The projector builds `DesktopRuntimeState` from listener, session, Space, sync, and update owners, publishes that state with `SetDesktopState`, then projects the same state into `DesktopTrayEntry` rows with `BuildDesktopTrayEntriesFromRuntimeState`.

The tray publisher accesses the host `DesktopTrayResourceService` through `AccessDesktopTray`, registers rows with `RegisterDesktopTrayEntry`, updates existing row resources with `SetDesktopTrayEntry`, and releases rows that are no longer present. Attached actions, such as `Install Update`, are caller-owned resources connected through the entry action transport.

The projector is process-lifetime code. It watches the listener status broker, session controller state, Space self-enrollment state, sync status, and launcher update state. It does not query Electron windows and it does not depend on an open renderer. When the publisher stops, it releases its registered tray entry resources from the host tray tree.

## Native Menu

The native menu is rendered by `bldr/web/electron/main/desktop-tray.ts`. It subscribes to `WatchDesktopTray`, ignores duplicate snapshots, and rebuilds the native menu only when the published entry tree changes.

Healthy mode contains:

- Spacewave status header.
- Open Spacewave and New Window commands.
- Status, Sessions, Spaces, Activity, and Quick Actions sections.
- Settings, About, and Quit.

Attention mode collapses the top of the menu toward the highest-priority actionable item while keeping Open Spacewave and Quit available.

Quick actions are intentionally conservative. Copy actions use the native clipboard. Reveal actions open the platform file manager. A staged native update publishes an attached `Install Update` tray action owned by the Spacewave launcher projector. Restart and recovery actions stay disabled until they have explicit confirmation and clean shutdown semantics.

## Popover Readiness

The native menu entry tree is the contract for tray commands. A richer popover may consume `DesktopRuntimeState` for presentation, but it must preserve the native menu as fallback and route commands through `DesktopTrayEntry` action transport or explicit desktop runtime actuators instead of inventing a second status or command model.

### DesktopTrayEntry Popover Contract

Popover renderers that dispatch tray commands consume the `DesktopTrayState.entries` tree as the command contract. `DesktopRuntimeState` supplies richer descriptive context, but the popover treats each `DesktopTrayEntry` as the source of truth for row identity, visibility, ordering, enabled state, and invocation semantics. The same snapshot also feeds the native menu, so a row that cannot be expressed through `DesktopTrayEntry` is not part of the desktop tray contract.

Stable identity comes from `DesktopTrayEntry.id`. Publishers keep the same ID for the same logical row across label, status, active, and enabled changes. Popovers use `id` as the reconciliation key and pass it unchanged to `InvokeDesktopTrayEntry` for attached-handler actions. An ID is unique within the tray tree while the entry is registered; when a publisher releases the entry resource, the row leaves the contract and any popover-local row state for that ID is discarded.

Display text comes from the entry fields, not from popover-side lookup tables. `label` is the primary visible name. `detail` is optional secondary text. `statusText` is compact row status. `iconName`, `iconState`, `severity`, and `active` are presentation hints for status, attention, and foreground activity. `SECTION`, `SEPARATOR`, `STATUS`, and `SUBMENU` entries are structural or informational and are not invoked.

Action rows are the only selectable command rows. A popover disables an action when `enabled` is false, when `action` is absent, or when the action payload is incomplete for its kind. It does not hide disabled rows unless the entry is removed from the watched tree. The supported action handoff is:

| `DesktopTrayAction.kind` | Popover behavior                                                                 |
| ------------------------ | --------------------------------------------------------------------------------- |
| `OPEN_ROUTE`             | Call `OpenOrFocusMainWindow` with `route` when present, otherwise focus the app.  |
| `NEW_WINDOW`             | Call `OpenOrFocusMainWindow` with the requested `route`, or `/` when empty.       |
| `COPY_TEXT`              | Copy `value` to the native clipboard.                                             |
| `REVEAL_PATH`            | Reveal `value` with the platform file manager.                                    |
| `QUIT`                   | Call `QuitDesktopRuntime`.                                                        |
| `ATTACHED_HANDLER`       | Call `InvokeDesktopTrayEntry` with the entry `id`; the publisher owns the handler. |

Ordering is the watched tree order. The tray service sorts registered entries by path, group, and order before publishing `DesktopTrayState`. Popovers render entries in the published order, preserve `path` as submenu or grouped hierarchy, and do not re-rank rows from `severity` or `active`; attention projection happens before publication.

Lifecycle follows the Resource SDK. Publishers register each row with `RegisterDesktopTrayEntry`, update it through the returned `DesktopTrayEntryResourceService`, and release the resource when the row no longer exists. Popovers subscribe with `WatchDesktopTray`, replace their local tree from each full snapshot, and treat missing rows as removed. They do not hold row resources, invoke released IDs, or keep commands from stale snapshots.

The native menu remains the required fallback. Electron main continues to rebuild and install the native menu from `WatchDesktopTray` even when a popover is enabled. If the popover cannot attach, render, or dispatch an action, the tray controller falls back to the native menu or opens/focuses the singleton app window; it must not route commands through a popover-only model.

The current custom popover is a desktop-only development prototype. Enable it with:

```bash
BLDR_ELECTRON_DESKTOP_TRAY_POPOVER=1 bun run start:desktop
```

When enabled, the tray left-click attempts to show the custom popover from the latest `DesktopRuntimeState`. The native context menu is still rebuilt and installed. If the popover window cannot attach or render, the controller disables the prototype for that run and falls back to opening or focusing the singleton Spacewave window.

Promotion requires screenshot and interaction checks on macOS, Windows, and Linux. Until that proof exists, the native menu remains the shipped tray surface.

## Platform Behavior

Platform parity means equivalent behavior, not identical visuals.

macOS uses a template menu bar icon when `macos_template_tray_icon_path` is configured. Electron can also set a short tray title fallback for icon state. Windows uses the notification area and tooltip/context menu behavior. Linux support depends on the active desktop environment's tray implementation.

The expected behavior is the same on all platforms:

- The tray/status item exists while the desktop runtime is running.
- Closing every Electron window leaves the runtime and CLI listener alive.
- Clicking the tray item or choosing Open Spacewave opens or focuses the singleton window.
- Launching the app again opens or focuses the singleton window instead of creating another runtime.
- Quit is the explicit path that tears down the tray-backed runtime.

## Debugging

Start the desktop app with:

```bash
bun run start:desktop
```

The tray Quick Actions section exposes `Copy Socket Path` when the listener socket path is known. `Copy Diagnostics` includes the collapsed status, listener detail, and socket path. Use those values as the first live check before starting a debugger: the copied socket should match the socket used by the CLI, and the diagnostics text should match the title row, listener row, and connected-client count in the native menu.

### CLI And Debug Verification

Use the CLI walkthrough commands against the copied socket path to verify the daemon surface from outside Electron:

```bash
bun run cli:local -- --socket-path /path/from/tray status
bun run cli:local -- --socket-path /path/from/tray whoami
bun run cli:local -- --socket-path /path/from/tray space list
```

`status` proves the daemon socket is reachable and reports the mounted session, lock state, and Space count. `whoami` proves which session identity the CLI is acting as. `space list` proves the session's Space entries are visible through the same daemon. For a non-default session, add `--session-index <n>` to each command.

When the debug bridge is enabled in a running desktop build, use the repo-local debug CLI to confirm the renderer bridge before evaluating page-side probes:

```bash
go run -mod=mod ./cmd/spacewave-debug wait
go run -mod=mod ./cmd/spacewave-debug info
```

The debug CLI talks to `.bldr/spacewave-debug.sock` or `SPACEWAVE_DEBUG_SOCK`. It is for renderer and plugin debug inspection. The authoritative native tray tree still lives in Electron main behind `DesktopTrayResourceService`.

### Resource Tree Inspection

Inspect the host `DesktopTray` tree with the Resource SDK tests before changing native menu behavior:

```bash
GOFLAGS=-mod=mod go test -count=1 ./bldr/desktop/tray
```

The important fixtures are:

| Test                                                                 | What it proves                                                                 |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `TestDesktopTrayRegistryWatchStreamsSnapshots`                       | `WatchDesktopTray` emits the current ordered `DesktopTrayState`.               |
| `TestDesktopTrayRegistryOrdersEntriesAndUpdatesState`                | registered entries sort by order/group and entry resources update active state. |
| `TestReconcileDesktopTrayMirrorsEntriesToTargetResource`             | source entries mirror into the Electron-owned target tray resource.             |
| `TestReconcileDesktopTrayMirrorsExistingEntriesAcrossTargetReconnect` | reconnecting a target receives existing source entries and old target rows drop. |

Inspect Electron main actuator state with:

```bash
bunx vitest run --config bldr/vitest.config.ts bldr/web/electron/main/desktop-runtime.test.ts
```

`DesktopRuntimeResource.WatchDesktopState` is the popover/debug stream to watch for `mainWindowOpen`, `quitting`, `statusText`, `health`, `lifecycle`, `listener`, `sessions`, `spaces`, `activity`, `update`, `attentionItems`, and `actions`. `SetDesktopState` only accepts projected runtime fields; Electron main preserves `mainWindowOpen` and `quitting`. `OpenOrFocusMainWindow` and `QuitDesktopRuntime` are the explicit desktop runtime actuators to verify for route/open and quit rows.

Inspect listener, session, and Space projection with:

```bash
GOFLAGS=-mod=mod go test -count=1 ./core/resource/desktop/statusprojector
```

`state_test.go` covers listener reachability states, session rows, Space rows, attention rows, update actions, bounded lists, and the `DesktopRuntimeState` to `DesktopTrayEntry` projection. `tray-publisher_test.go` covers publishing projected tray entries into the host tray resource and releasing removed actions.

Inspect the native Electron menu rendering with:

```bash
bunx vitest run --config bldr/vitest.config.ts bldr/web/electron/main/desktop-tray.test.ts
```

Use this when labels, route actions, copy actions, popover fallback, or menu rebuild behavior changes. The tests assert the daemon-console order, the synthetic `Copy Socket Path` and `Copy Diagnostics` rows, route dispatch through `OpenOrFocusMainWindow`, attached-handler dispatch through `InvokeDesktopTrayEntry`, and duplicate snapshot suppression.

Use focused tests while changing the daemon console:

```bash
bunx vitest run --config bldr/vitest.config.ts bldr/web/bldr/web-runtime.test.ts bldr/web/electron/main/desktop-runtime.test.ts bldr/web/electron/main/desktop-tray.test.ts
GOFLAGS=-mod=mod go test -count=1 ./bldr/desktop/tray ./bldr/web/runtime ./bldr/web/electron/desktop-runtime ./core/resource/desktop/statusprojector ./core/resource/session
```

The opt-in Electron e2e suite is heavier and only runs when enabled:

```bash
ENABLE_E2E_ELECTRON=true GOFLAGS=-mod=mod go test -count=1 ./e2e/electron
```

Popover promotion evidence uses the same opt-in harness with the popover flag
set before the harness boots:

```bash
ENABLE_E2E_ELECTRON=true BLDR_ELECTRON_DESKTOP_TRAY_POPOVER=1 GOFLAGS=-mod=mod go test -count=1 ./e2e/electron
```

The Electron-main e2e control surface exposes `POST /tray-popover/open` and
`GET /tray-popover/screenshot` for screenshot capture from the current
`DesktopTrayState`. The Go harness wraps these as `OpenTrayPopover` and
`CaptureTrayPopoverScreenshot`, writing PNG files under the harness artifact
directory. These controls are test-only; the native menu remains the default
runtime tray surface.

`TestDesktopDaemonConsoleKeepsCLIReachableWithoutWindows` installs
`./cmd/spacewave` into a temporary `GOBIN`, invokes the command by name through
`PATH`, closes every Electron renderer page, and runs
`spacewave --output json status --socket-path <desktop-socket>`. The expected
result is a running JSON status report containing the desktop socket path. This
is the CLI-level proof that the installed command talks to the tray-backed
runtime without an open window.

For projector issues, inspect the status projector tests first. For native menu issues, inspect `desktop-tray.test.ts` before starting a full desktop harness.
