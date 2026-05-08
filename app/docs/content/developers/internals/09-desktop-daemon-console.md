---
title: Desktop Daemon Console
section: internals
order: 9
summary: Electron main tray state, background runtime behavior, and desktop status debugging.
---

## Overview

Spacewave desktop keeps a native status surface alive for the process lifetime. On macOS this is the menu bar extra. On Windows it is the notification-area icon. On Linux it is the desktop environment's tray/status item when the environment exposes one.

The tray is a daemon console, not a second app shell. It shows bounded runtime state, opens or focuses the singleton Spacewave window, exposes safe diagnostics, and provides explicit quit. Closing every Electron window does not stop the background runtime when the project uses tray-backed desktop presence.

## State Tree

Electron main owns the generic desktop runtime Resource SDK tree in `bldr/web/electron/main/desktop-runtime.ts`. The root service is `DesktopRuntimeResourceService`:

| RPC                     | Purpose                                                                                         |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| `WatchDesktopState`     | Streams the latest `DesktopRuntimeState` to native tray renderers and process-lifetime clients. |
| `SetDesktopState`       | Publishes projected status from the runtime side into Electron main.                            |
| `OpenOrFocusMainWindow` | Opens or focuses the singleton app window, optionally at a route.                               |
| `QuitDesktopRuntime`    | Marks the runtime as quitting and requests an explicit shutdown.                                |

`DesktopRuntimeState` contains the tray-visible contract:

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

Spacewave-specific interpretation lives in `core/resource/desktop/statusprojector`. The projector connects to the Electron-main Resource SDK tree through `ConnectDesktopRuntimeResourceClient`, accesses the root resource, and publishes `DesktopRuntimeState` with `SetDesktopState`.

The projector is process-lifetime code. It watches the listener status broker, session controller state, Space self-enrollment state, sync status, and launcher update state. It does not query Electron windows and it does not depend on an open renderer. On teardown it publishes a deterministic disconnected state unless Electron main has already moved into explicit quit.

## Native Menu

The native menu is rendered by `bldr/web/electron/main/desktop-tray.ts`. It subscribes to `WatchDesktopState`, ignores duplicate snapshots, and rebuilds the native menu only when the state changes.

Healthy mode contains:

- Spacewave status header.
- Open Spacewave and New Window commands.
- Status, Sessions, Spaces, Activity, and Quick Actions sections.
- Settings, About, and Quit.

Attention mode collapses the top of the menu toward the highest-priority actionable item while keeping Open Spacewave and Quit available.

Quick actions are intentionally conservative. Copy actions use the native clipboard. Reveal actions open the platform file manager. Restart and recovery actions stay disabled until they have explicit confirmation and clean shutdown semantics.

## Popover Readiness

The native menu state is the contract for any richer popover. A popover must consume the same `DesktopRuntimeState` stream that drives the native menu, preserve the native menu as fallback, and route commands through `DesktopRuntimeResource` instead of inventing a second status or command model.

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

The tray Quick Actions section exposes `Copy CLI Socket` when the listener socket path is known. `Copy Diagnostics` includes the collapsed status, listener detail, and socket path.

Use focused tests while changing the daemon console:

```bash
bunx vitest run --config bldr/vitest.config.ts bldr/web/bldr/web-runtime.test.ts bldr/web/electron/main/desktop-runtime.test.ts bldr/web/electron/main/desktop-tray.test.ts
GOFLAGS=-mod=mod go test -count=1 ./bldr/web/runtime ./bldr/web/electron/desktop-runtime ./core/resource/desktop/statusprojector ./core/resource/session
```

The opt-in Electron e2e suite is heavier and only runs when enabled:

```bash
ENABLE_E2E_ELECTRON=true GOFLAGS=-mod=mod go test -count=1 ./e2e/electron
```

`TestDesktopDaemonConsoleKeepsCLIReachableWithoutWindows` installs
`./cmd/spacewave` into a temporary `GOBIN`, invokes the command by name through
`PATH`, closes every Electron renderer page, and runs
`spacewave --output json status --socket-path <desktop-socket>`. The expected
result is a running JSON status report containing the desktop socket path. This
is the CLI-level proof that the installed command talks to the tray-backed
runtime without an open window.

For projector issues, inspect the status projector tests first. For native menu issues, inspect `desktop-tray.test.ts` before starting a full desktop harness.
