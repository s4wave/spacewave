---
title: Startup Manifest Recovery
section: internals
order: 10
summary: Retained-state startup manifest validation, smoke tests, artifacts, and recovery boundaries.
---

## Overview

Spacewave validates plugin startup manifests before launching plugin-backed
desktop views. A retained desktop state root keeps cached devtool manifests
available for startup validation, and the launcher preflights those manifests
before the plugin scheduler starts. When the cached build is still valid, startup
reuses the cached manifest build instead of waiting for a fresh build during the
launcher path.

The retained-state recovery boundary is the app data root plus the Electron user
data root used for a launch pair. A retained startup smoke runs an initial launch
and a retained launch against the same roots. Passing proof means the retained
launch reaches Electron/app startup while reusing that state. It does not prove
cloud sync, account recovery, update installation, or every plugin runtime
operation.

## Issue Note

The startup-manifest issue is visible when an app that already has retained
state cannot reach the shell or Electron startup because plugin startup
manifests are missing, stale, or unavailable during startup. The expected
current behavior is:

- The initial launch populates launcher, app data, and devtool startup manifest
  state.
- The retained launch keeps cached devtool manifests for validation.
- The launcher preflights startup manifests before the scheduler starts.
- Valid cached startup manifest builds are reused during startup preflight.
- A failed retained launch writes diagnostics before the test exits.

Treat a failure as a retained-state startup regression until the captured
artifacts show a narrower cause. Do not silently delete retained state as a test
fix; deleting state changes the boundary being verified.

## SOP

Use the focused Go tests first when changing startup manifest validation,
startup manifest collection, or plugin scheduler handling:

```bash
GOFLAGS=-mod=mod go test -count=1 ./bldr/manifest/builder/controller ./bldr/manifest/world ./bldr/plugin/host/scheduler
```

Use the Electron retained-state smoke for the devtool launcher path:

```bash
ENABLE_E2E_ELECTRON=true GOFLAGS=-mod=mod go test -count=1 -run TestRetainedStateLauncherStartupSmoke ./e2e/electron
```

Use the packaged installed-app smoke for a signed/native app bundle or packaged
binary. On macOS, `SPACEWAVE_INSTALLED_APP_PATH` is usually an `.app` bundle:

```bash
ENABLE_E2E_INSTALLED_APP=true \
SPACEWAVE_INSTALLED_APP_PATH=/Applications/Spacewave.app \
SPACEWAVE_INSTALLED_APP_STATE_ROOT=/tmp/spacewave-installed-retained \
GOFLAGS=-mod=mod go test -count=1 -run TestPackagedInstalledAppRetainedStateLauncherStartupSmoke ./e2e/installedapp
```

Keep `SPACEWAVE_INSTALLED_APP_STATE_ROOT` dedicated to this smoke. The test
clears that root before the initial launch and then reuses it for the retained
launch.

## Test Notes

`TestRetainedStateLauncherStartupSmoke` writes artifacts under the Electron
harness artifact directory. The success breadcrumb is
`retained-startup-breadcrumbs.txt`; log tails are
`retained-startup-initial-log-tail.txt` and
`retained-startup-retained-log-tail.txt`. On failure, the test writes
`retained-startup-failure-diagnostics.txt` plus any available failure log tails.

The Electron smoke expects these retained-launch log proofs:

- `keeping cached devtool manifests for startup validation`
- `preflighting startup manifests`
- `reused cached startup manifest build`

`TestPackagedInstalledAppRetainedStateLauncherStartupSmoke` writes artifacts
under `$SPACEWAVE_INSTALLED_APP_STATE_ROOT/artifacts`. The success breadcrumb is
`installed-app-retained-startup-breadcrumbs.txt`; the full logs are
`installed-app-initial.log` and `installed-app-retained.log`; log tails are
`installed-app-initial-log-tail.txt` and
`installed-app-retained-log-tail.txt`.

The packaged smoke expects the retained launch to log `initializing application
and storage` and `starting electron:` while using the same app data and Electron
user data roots as the initial launch. On macOS, it also verifies the installed
app signature before launching.

## Recovery Boundary

The retained-state startup proof is bounded to local startup manifest recovery.
It proves that cached startup manifest state survives an app relaunch and is
validated early enough for plugin-backed startup to continue. It does not make
network calls, mutate live cloud services, deploy releases, or prove account
recovery.

When diagnosing a failure, preserve the artifact directory and state root until
the logs identify the broken layer. Escalate only after capturing the breadcrumb,
the initial and retained log paths, both log tails, and the exact command line
used for the smoke.
