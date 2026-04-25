# Memlab WASM E2E Tests

This package adds heap-snapshot analysis on top of Alpha's `e2e/wasm` browser
test harness.

It boots the real WASM app, captures Chrome V8 heap snapshots through the
DevTools Protocol, and analyzes retained JavaScript objects with
`@memlab/heap-analysis`.

## Scope

The memlab tests are intended for browser-side retention checks around the
WASM app runtime and its JS transport layers.

They currently track:

- `ClientRPC`
- `ChannelStream`
- `Promise`
- `Generator`
- `onNext` closures

For retained `ClientRPC` objects, the analyzer also groups counts by
`service/method` pair.

## What It Measures

These tests measure the page's V8 heap.

They do not directly measure:

- dedicated worker heaps
- ServiceWorker heaps
- Go WASM linear memory
- CPU usage

Use this package when the question is whether browser-side JS objects are being
retained across route changes, idle periods, or watch lifecycles.

## Package Layout

- `memlab_test.go`
  - scenario definitions
- `snapshot.go`
  - heap snapshot capture via CDP `HeapProfiler`
- `capture.go`
  - labeled snapshot set management
- `analyze.js`
  - heap analysis and object counting
- `analyze-runner.go`
  - ordered invocation of the JS analyzer from Go
- `assert.go`
  - threshold-based test assertions
- `testdata/`
  - captured `.heapsnapshot` files for each scenario

## Installation

The Go tests install the package-local Node dependencies automatically on first
run if `node_modules/` is missing.

To install them manually:

```bash
cd e2e/wasm/memlab
bun install
```

## Running The Tests

From the repository root:

Run the full memlab suite:

```bash
go test -v -timeout=45m ./e2e/wasm/memlab/...
```

Run a single scenario:

```bash
go test -v -timeout=20m ./e2e/wasm/memlab/... -run 'TestDriveScenario$'
```

```bash
go test -v -timeout=20m ./e2e/wasm/memlab/... -run 'TestWatchCleanupScenario$'
```

```bash
go test -v -timeout=20m ./e2e/wasm/memlab/... -run 'TestIdleBaselineScenario$'
```

```bash
go test -v -timeout=20m ./e2e/wasm/memlab/... -run 'TestDriveIdleGrowthScenario$'
```

## Scenarios

### `TestDriveScenario`

Captures snapshots across the quickstart drive flow:

- baseline on landing
- snapshot after drive loads
- cleanup snapshot after navigating away

Use this for mount/unmount retention across the drive route.

### `TestWatchCleanupScenario`

Captures snapshots around the session dashboard route:

- baseline on landing
- cleanup snapshot after entering and leaving the session dashboard

Use this for retention caused by dashboard- and session-scoped watchers.

### `TestIdleBaselineScenario`

Captures snapshots on the landing page with no route change:

- immediate baseline
- idle snapshot after 30 seconds

Use this for steady-state landing-page accumulation checks.

### `TestDriveIdleGrowthScenario`

Captures a small time series after drive is fully loaded:

- load `#/quickstart/drive`
- wait for `WaitForDriveReady`
- capture `t1`
- wait 15 seconds
- capture `t2`
- wait 15 seconds
- capture `t3`

Use this to distinguish ongoing idle growth from steady-state retained objects
after the drive screen has settled.

## Output

Snapshots are written under:

```text
e2e/wasm/memlab/testdata/<TestName>/
```

Typical files include:

- `baseline.heapsnapshot`
- `action.heapsnapshot`
- `cleanup.heapsnapshot`
- `t1.heapsnapshot`
- `t2.heapsnapshot`
- `t3.heapsnapshot`

Test logs include:

- aggregate retained object counts
- top retained `ClientRPC` service/method pairs
- pair deltas between the first and last snapshot

`TestDriveIdleGrowthScenario` also logs per-snapshot counts so time-series
behavior is visible directly in the test output.

## Analyzer Semantics

The analyzer compares the last snapshot in a scenario against the first
snapshot in that scenario.

Examples:

- `baseline -> cleanup`
- `baseline -> action`
- `t1 -> t3`

Snapshot order is preserved by the Go runner before invoking `analyze.js`.

## Interpreting Results

A failure means one or more retained object deltas exceeded the thresholds in
`assert.go`.

Typical output includes lines such as:

- `LEAK: ClientRpc delta 19 exceeds threshold 2`
- `ok: ChannelStream delta 0 (threshold 2)`

The most useful follow-up signals are usually:

- top retained `ClientRPC` service/method pairs
- top pair deltas between first and last snapshot
- per-snapshot counts in time-series scenarios

Flat counts across multiple snapshots suggest steady-state retention. Rising
counts across snapshots suggest ongoing accumulation.

## Related Packages

- `e2e/wasm/`
  - shared browser/WASM harness used by these tests
- `app/`, `web/`, and SDK/resource layers
  - common sources of retained transport or watch state surfaced by the pair
    breakdowns
