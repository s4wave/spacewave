# Shared-daemon desktop fixtures

`TestSharedDaemonElectron/native-core` and
`TestSharedDaemonElectron/distribution-core` run the production Electron main,
preload, runtime pipe, and desktop controller with a small renderer and a real
DedicatedWorker. Each fixture creates one local Session and Space through the
protected CLI socket, compares their identities with the rendered window, and
renames the Space from the worker. The same CLI watch receives worker changes
before and after window close/reopen, plus a CLI change while the shell is closed.
Both shell generations must exit cleanly.

The native fixture resolves the local Resource service. The distribution fixture
forwards through the production core-plugin RPC path, with the core implementation
in-process. This checks both routing contracts, not a packaged distribution or
plugin artifact selection.

## Run on Linux

Requires Go, Bun, Xvfb, and an Electron executable. From the repository root:

```sh
go mod vendor
go run -p=2 ./e2e/shareddaemon/prepare /tmp/shared-daemon-app
export SPACEWAVE_SHARED_DESKTOP_FIXTURE=/tmp/shared-daemon-app
export SPACEWAVE_SHARED_DESKTOP_ELECTRON=/path/to/electron
bash e2e/shareddaemon/run.sh
```

Preparation bundles production main/preload and fixture renderer/worker sources.
It runs separately from the short runtime checks. `GO_TEST_JOBS` controls Go build
parallelism; `SPACEWAVE_SHARED_DESKTOP_DEBUG=1` enables Electron and daemon logs.
Without the two fixture paths, ordinary package tests skip these opt-in fixtures.
The fixtures never attach to a saved profile or an existing daemon.
