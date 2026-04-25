# Testbed Infrastructure

This directory contains the testbed infrastructure for running tests against the s4wave backend.

## Directory Structure

```
testbed/
├── browser/           # Browser test server infrastructure
│   ├── server.go      # Base WebSocket RPC server
│   └── layout-server.go # Layout-specific server with LayoutHost service
├── testbed.go         # World testbed with engine setup
└── option.go          # Testbed options
```

## Browser Test Servers

### `browser/server.go` - Base WebSocket Server

Generic WebSocket-based RPC server that wraps any `srpc.Mux`. Used as the foundation for all browser E2E tests.

```go
mux := srpc.NewMux()
// Register services on mux...
server := browser_testbed.NewServer(le, mux)
port, err := server.Start(ctx)
defer server.Stop(ctx)
```

### `browser/layout-server.go` - Layout Server

Specialized server for layout-only browser tests. Includes:

- `LayoutHost` service registration
- State management via `broadcast.Broadcast`
- Methods for waiting on frontend updates (`WaitForLayoutUpdate`, `WaitForNavigateTab`)

```go
server := browser_testbed.NewLayoutServer(le)
server.SetLayoutModel(model) // Server-initiated model updates
port, err := server.Start(ctx)
updated, err := server.WaitForLayoutUpdate(ctx) // Wait for frontend updates
```

## Test Locations

### `core/resource/testbed/` - Resource SDK Tests

Go unit tests and browser E2E tests for the Resources SDK:

| File                     | Description                                            |
| ------------------------ | ------------------------------------------------------ |
| `testbed_test.go`        | Full integration tests                                 |
| `testbed_simple_test.go` | Simple unit tests                                      |
| `testbed_e2e_test.go`    | Resources SDK E2E tests                                |
| `browser-e2e_test.go`    | Browser E2E tests for `web/layout/`                    |
| `browser-server.go`      | Deprecated wrapper, use `browser_testbed.LayoutServer` |

### `core/resource/layout/testbed/` - Layout Unit Tests

Go unit tests for the layout resource implementation:

| File             | Description                                                          |
| ---------------- | -------------------------------------------------------------------- |
| `testbed.go`     | Layout testbed setup with resource client                            |
| `layout_test.go` | Layout resource unit tests (WatchLayoutModel, SetModel, NavigateTab) |

### `core/e2e/` - Full Backend E2E Tests

Browser E2E tests with full bldr backend (plugins, compilers, etc.):

| File                          | Description                                                          |
| ----------------------------- | -------------------------------------------------------------------- |
| `e2e_test.go`                 | TypeScript test runner                                               |
| `browser/browser-e2e_test.go` | Full app browser tests (`web/app/App.backend.e2e.test.tsx`)          |
| `browser/browser-server.go`   | Full backend browser server (wraps resource mux with ResourceServer) |

## Test Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Browser Tests (vitest)                        │
│  web/layout/*.test.tsx          web/app/App.backend.e2e.test.tsx    │
└───────────────────────────────────┬─────────────────────────────────┘
                                    │ WebSocket
                                    ▼
┌───────────────────────────────────────────────────────────────────────┐
│                    testbed/browser/Server                              │
│                    (WebSocket RPC Server)                              │
└───────────────────────────────────┬───────────────────────────────────┘
                                    │
        ┌───────────────────────────┴───────────────────────────┐
        ▼                                                       ▼
┌─────────────────────────┐                       ┌─────────────────────────┐
│  LayoutServer           │                       │  BrowserTestServer      │
│  (layout-only tests)    │                       │  (full backend tests)   │
│                         │                       │                         │
│  - LayoutHost service   │                       │  - ResourceServer       │
│  - broadcast.Broadcast  │                       │  - TestbedResourceServer│
│    for state sync       │                       │  - Full bldr backend    │
└─────────────────────────┘                       └─────────────────────────┘
```

## Usage Patterns

### Layout-Only Browser Tests

```go
// In core/resource/testbed/browser-e2e_test.go
server := browser_testbed.NewLayoutServer(le)
helper := resource_testbed.NewLayoutServerHelper(server)
helper.SetupInitialLayoutModel()

port, _ := server.Start(ctx)
defer server.Stop(ctx)

// Run vitest with port
cmd := exec.CommandContext(ctx, "bun", "test:browser:layout")
cmd.Env = append(os.Environ(), fmt.Sprintf("VITE_E2E_SERVER_PORT=%d", port))
```

### Full Backend Browser Tests

```go
// In core/e2e/browser/browser-e2e_test.go
testbedResourceServer := resource_testbed.NewTestbedResourceServer(ctx, le, b, volumeID, bucketID)
rootResourceMux := srpc.NewMux()
testbedResourceServer.Register(rootResourceMux)

browserServer := s4wave_core_e2e_browser.NewBrowserTestServer(le, b, rootResourceMux)
port, _ := browserServer.Start(ctx)
defer browserServer.Stop(ctx)

// Run vitest with port
cmd := exec.CommandContext(ctx, "bun", "test:browser:app")
cmd.Env = append(os.Environ(), fmt.Sprintf("VITE_E2E_SERVER_PORT=%d", port))
```

### Go Unit Tests

```go
// In core/resource/layout/testbed/layout_test.go
tb, err := layout_testbed.Default(ctx)
defer tb.Release()

setup, err := tb.SetupLayoutEngine(ctx, objectKey)
defer setup.Release()

// Use setup.LayoutResourceID to access the layout resource
```

## State Synchronization

The `LayoutServer` uses `broadcast.Broadcast` for thread-safe state synchronization:

```go
// Writing state and notifying waiters
s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
    s.lastLayoutUpdate = layoutModel
    broadcast() // Wake all waiters
})

// Waiting for state changes
result, err := s.bcast.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
    if s.lastLayoutUpdate != nil {
        result = s.lastLayoutUpdate
        s.lastLayoutUpdate = nil
        return true, nil // Done waiting
    }
    return false, nil // Keep waiting
})
```

This pattern is preferred over raw channels for state synchronization. See AGENTS.md for details.
