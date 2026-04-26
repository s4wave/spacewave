## Spacewave Agent Guide

This file contains rules specific to `repos/spacewave`. Follow the shared
company AGENTS rules first, then apply these Spacewave-specific rules.

## Repository Basics

- Work from the repository root unless a command explicitly belongs in a
  subdirectory.
- Do not access paths outside the current working directory and its
  subdirectories.
- Do not assume `bldr setup` needs to be run manually. Bldr runs setup
  automatically for almost any operation.
- Do not assume Tailwind utility pixel sizes such as `h-4` or `h-16`; they
  depend on `var(--spacing)`.
- Styling uses Tailwind v4 with theme variables in `web/style/app.css`.
- Do not add dead-code fallback paths for impossible conditions. If a field is
  guaranteed non-nil by construction, do not add defensive nil branches in
  methods. These fallbacks mask bugs and rot into false invariants.
- Docs describe the current system, not the migration path. Use direct
  present-state wording in docs, design notes, and tracker summaries.

## Go Modules And Vendoring

- When adding `replace` directives to `go.mod`, use absolute paths such as
  `replace github.com/foo/bar => /absolute/path/to/company/repos/bar`. Relative
  paths break when the working directory changes.
- After any `go.mod` change, including dependencies, replace directives, or
  version bumps, run:

  ```bash
  go mod tidy && go mod vendor
  ```

- Keep `vendor/` synchronized with `go.mod`.

## RPC, Cache, And Resource Lifecycles

### Streaming State

- SDK RPCs that return mutable state must be server-streaming `Watch*` RPCs, not
  unary `Get*` RPCs.
- The UI uses `useStreamingResource` for server-pushed updates.
- Any state that can change from the CLI, another tab, or a background process
  must be reactive.
- Unary RPCs are appropriate only for immutable values such as session refs and
  peer IDs, or for one-shot actions such as create, delete, and set.
- If you are adding a `Get*` RPC that returns mutable state, make it a `Watch*`
  RPC.

### Proto3 Bool Fields In TypeScript

Proto3 omits default values from the wire format. `protobuf-es-lite` leaves
omitted bool fields as `undefined` after deserialization.

- Check proto bools with `field ?? false` or `!!field`.
- Do not use `field === undefined` to detect "not yet loaded".
- For loading state, check whether the containing message is `null`.

Example: `useStreamingResource` returns `value: null` before the first emission
and `value: {}` after emitting `{setupRequired: false}`. Check `value` for null,
not `value.setupRequired` for undefined.

### Resource IDs

RPCs that return `resource_id` values allocate server-side resources with cleanup
callbacks.

- Wrap returned IDs with `resourceRef.createRef(id)` to create a
  `ClientResourceRef`.
- Release refs when the caller is done with them.
- In a fixed async scope, bind each wrapped ref to its own `using` declaration.
- Use cleanup stacks only for dynamic or cross-helper lifetimes.
- Never discard resource IDs with `void resourceId`.
- Do not add `Unregister*` or `Remove*` RPCs for resources that already use
  resource-based lifecycle. Release the resource instead.

### useResource Released-Resource Retry

`useResource(...)` retries by default when the loaded value is an SDK `Resource`
and the client emits `server-released` for that resource ID.

- Use `retryOnReleasedResource: false` only when server release is expected and
  terminal.
- For composite or non-`Resource` return values, pass
  `retryOnReleasedResource: { getResourceIds: ... }`.

### Cloud-Backed Mutable State

- Never make redundant cloud HTTP requests.
- Account state such as keypairs, account info, and thresholds must be fetched
  once when the session mounts and cached locally in the Go provider's
  ObjectStore.
- Go Watch loops such as `WatchAccountInfo` and `WatchAuthMethods` serve cached
  data to the UI through local SRPC.
- Cloud data is refetched only when invalidated by a hash change in the
  session/register response or by a Session DO WebSocket notify message.
- Multiple React components subscribing to the same Watch stream must share one
  Go-side stream.
- Mutable cloud-backed UI state must use this shape:

  ```text
  UI -> SRPC/watch -> Go cache/tracker state -> cloud sync machinery
  ```

- The UI must not trigger cloud fetches just to render current state.
- If a screen needs mutable cloud-backed state, first add or reuse a Go cache
  owner such as `ProviderAccount`, a session tracker, or a per-SO tracker.
- Known-gated owner-only cloud calls must check cached subscription/lifecycle
  state on the client and short-circuit locally when the account is inactive,
  read-only, or dormant.

### Watch Ownership

- Own mutable watches at container boundaries.
- Expose mutable state to the UI through Watch RPCs and subscribe at the nearest
  route/container, such as `SessionContainer`, `SpaceContainer`, or an org
  container.
- Pass watched snapshots down through React context.
- Do not start separate mutable watches or fetches in leaf components.
- Batch related low-churn state into one combined watch per screen/domain.
- Do not create one giant watch that couples unrelated high-churn state.

### Resource Wrapper State

Do not attach shared mutable or persistent state to per-client Resource wrappers.
Resource handles returned by `Mount*` and `Access*` RPCs are client-specific
wrappers and may be recreated multiple times for the same underlying session,
account, or object.

Shared state owners such as caches, broadcasts, refcounts, and object-store
managers must live on shared domain objects or shared registries keyed by stable
identity. Resource wrappers should forward into the shared owner.

## Backend Patterns

### Controller Registration

Controllers are almost never registered by calling `AddFactory` directly in Go
production code. Register controllers through `bldr.yaml` configSet entries:

1. Add the controller's Go package to `goPkgs` in the manifest builder config.
2. Add a `configSet` entry with the controller's `ConfigID` and config fields.
3. At build time, the Go compiler scans `goPkgs` for `NewFactory` and
   `BuildFactories` functions and generates a `plugin.go` with a `Factories`
   array.
4. At runtime, the plugin registers all factories and deserializes
   `config-set.bin`, matching each `id` to a factory's `ConfigID`.

Direct `AddFactory` calls belong in tests, such as `core/e2e/e2e_test.go`.

### JSON

- Do not use `encoding/json`.
- For proto messages, use generated `MarshalJSON`/`UnmarshalJSON` or
  `MarshalProtoJSON`/`UnmarshalProtoJSON` from `protobuf-go-lite/json`.
- For non-proto HTTP request/response structs, use
  `aperturerobotics/fastjson`.
- Cloud API endpoints define proto messages in
  `core/provider/spacewave/api/api.proto` and use the generated binary codec
  (`MarshalVT` / `UnmarshalVT`) on both sides. Do NOT use `MarshalJSON` /
  `UnmarshalJSON` or `MarshalProtoJSON` / `UnmarshalProtoJSON` for the cloud
  surface. See the "Cloud HTTP Client" rules below.
- For raw JSON passthrough, use a `string` proto field for opaque JSON strings,
  or `[]byte` plus fastjson for non-proto raw JSON handling.

### Cloud HTTP Client

All HTTP traffic to `repos/spacewave-cloud` goes through
`core/provider/spacewave/client.go`. The approved helpers for cloud calls are:

- `doPostBinary(ctx, path, reqProto)` for POST requests with proto-binary
  bodies and proto-binary responses
- `doGetBinary(ctx, path)` for GET requests returning proto-binary responses
- `doDelete(ctx, path)` for DELETE requests
- `doPostStream(ctx, path, reqProto)` for streaming responses (sync pull, etc.)
- `doMultiSig(ctx, path, action)` for multi-sig action requests; the response
  unmarshals into `MultiSigActionResponse`

Required behaviour for any cloud call:

- request body is `proto.MarshalVT(value)` with `Content-Type:
  application/octet-stream`
- response body is parsed with `proto.UnmarshalVT(value)`
- every cloud endpoint has both a request proto and a response proto in
  `core/provider/spacewave/api/`, including pure acks (typed-but-empty
  messages)

Streaming binary responses are an exception to the proto-binary response
rule. Routes that stream bulk bytes from the cloud (packfile downloads,
release artifact downloads, R2 object passthrough, anything where the
cloud streams from R2) carry a raw byte stream as their wire contract,
not a proto schema. Read these via `doPostStream` / a streaming GET helper
and consume the response body with `io.Copy` into the destination
sink rather than buffering the full body and decoding a proto.

Forbidden in cloud client code:

- `doPostJSON` (removed; previous proto-JSON helper)
- `aperturerobotics/fastjson` for cloud requests or responses
- `MarshalJSON` / `UnmarshalJSON` / `MarshalProtoJSON` / `UnmarshalProtoJSON`
  on proto types crossing the spacewave <-> cloud boundary
- hand-rolled JSON request bodies, hand-parsed JSON response bodies

WebSocket frames received from the cloud on the spacewave <-> cloud boundary
are
binary frames carrying a per-endpoint envelope proto with a oneof body case
(`WsAuthSessionServerFrame`, `WsBillingCheckoutServerFrame`). Parse with
`UnmarshalVT` and switch on the oneof. Do NOT call `UnmarshalJSON` on cloud
WS frames.

### HTTP Response Bodies

In Go HTTP client code, drain unread response body bytes to `io.Discard` before
closing the body. This preserves keep-alive connection reuse.

If the code reads the full body with `io.ReadAll`, `readResponseBody`, or a
streaming copy to EOF, close the body normally.

### No Fire-And-Forget Goroutines

Never spawn a goroutine from a callback, event handler, WebSocket frame handler,
or other hot path using `context.Background()` for detached background work.

Use `util/routine.RoutineContainer`, or `StateRoutineContainer` when work should
run only in a particular state. The owning long-lived component owns the
lifecycle context and cancels it on close. The callback triggers the routine; it
does not run the work itself.

Pattern:

1. Add a lifecycle `ctx context.Context` and `ctxCancel context.CancelFunc` to
   the owner. Cancel it in the close path.
2. Construct a `routine.RoutineContainer`.
3. Call `SetRoutine` with the function that performs the work. The routine
   receives the derived context from `SetContext`.
4. Call `SetContext(o.ctx, true)` once to wire lifecycle.
5. In callback paths, call `RestartRoutine()`.
6. In the close path, call `ClearContext()` and then cancel the lifecycle
   context.

```go
type Owner struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	refresh   *routine.RoutineContainer
	release   func()
}

func NewOwner(le *logrus.Entry) *Owner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Owner{ctx: ctx, ctxCancel: cancel}
}

func (o *Owner) wireRefresh(bs *blockStore, so *sharedObject) {
	o.refresh = routine.NewRoutineContainerWithLogger(o.le)
	o.refresh.SetRoutine(func(ctx context.Context) error {
		bs.Invalidate()
		return so.RefreshSnapshot(ctx)
	})
	o.refresh.SetContext(o.ctx, true)

	o.release = provider.RegisterCallback(func(id string) {
		if id != targetID {
			return
		}
		o.refresh.RestartRoutine()
	})
}

func (o *Owner) Close() {
	if o.release != nil {
		o.release()
	}
	if o.refresh != nil {
		o.refresh.ClearContext()
	}
	o.ctxCancel()
}
```

This rule applies to every case where anonymous goroutines with
`context.Background()` look convenient. Use lifecycle-scoped primitives from
`util/`, including `routine`, `keyed`, `refcount`, and `broadcast`.

### RefCount For Shared Background Goroutines

When multiple RPC subscribers need to share one background goroutine, such as a
WebSocket connection, use `refcount.RefCount` from `util/refcount`.

Pattern:

1. Store shared state behind a `broadcast.Broadcast`.
2. Create a `refcount.RefCount[struct{}]` whose resolver is the background
   goroutine.
3. Each RPC subscriber calls `AddRef`, waits on the broadcast for state changes,
   and calls `Release` when done.
4. Call `SetContext` with the parent lifecycle context.

```go
type parent struct {
	statusBcast broadcast.Broadcast
	status      string
	ticket      string
	statusRc    *refcount.RefCount[struct{}]
}

func (p *parent) resolveStatusWatcher(
	ctx context.Context,
	released func(),
) (struct{}, func(), error) {
	var ticket string
	p.statusBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ticket = p.ticket
	})
	if ticket == "" {
		return struct{}{}, nil, errors.New("no ticket")
	}

	err := p.runWatcher(ctx, ticket)
	return struct{}{}, nil, err
}

func (s *Resource) WatchStatus(
	req *WatchReq,
	strm WatchStream,
) error {
	ctx := strm.Context()
	parent := s.getParent()

	ref := parent.statusRc.AddRef(nil)
	defer ref.Release()

	var prev string
	for {
		var ch <-chan struct{}
		var status string
		parent.statusBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			status = parent.status
		})
		if status != prev {
			if err := strm.Send(&Resp{Status: status}); err != nil {
				return err
			}
			prev = status
		}
		if isTerminal(status) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}
```

Use `KeyedRefCount` from `util/keyed` when multiple independent goroutines are
keyed by ID.

## Data Model And Identifier Rules

### SharedObject And Block Store IDs

SharedObject IDs are ULIDs: 26 lowercase Crockford base32 characters. The block
store ID for a SharedObject-backed block store equals the SharedObject ULID
verbatim.

- `SobjectBlockStoreID(soID)` in `core/provider/local/id.go` and
  `core/provider/spacewave/sobject.go` returns `soID` directly. Use the helper
  for call-site clarity.
- Never prefix a SharedObject ULID to form a block store ID.
- Do not introduce translation helpers like `cloudResourceID(bstoreID)` or
  `soIDFromBstoreID(bstoreID)`.
- On the cloud side, the `bstoreId` URL parameter equals `soID`.
- The same rule applies to other ULID-keyed resources: store the ULID verbatim.
  Use separate ID columns or typed wrappers for disambiguation.

### Volume IDs

When calling `volume.ExBuildObjectStoreAPI`, the `volumeID` parameter must be
the mounted volume's ID from `vol.GetID()`, never a raw `StorageVolumeID()`
string.

The bldr plugin host proxies volumes through an RPC layer that changes volume
IDs. A proxy volume on the plugin bus has the bolt volume ID, such as
`hydra/volume/bolt/12D3KooW...`, not the original storage volume ID, such as
`p/local/{accountID}`. Any `ExBuildObjectStoreAPI` call using a raw storage
volume ID can hang because alias matching does not find the proxy volume.

Correct:

```go
volume.ExBuildObjectStoreAPI(ctx, bus, false, objStoreID, vol.GetID(), cancel)
```

Wrong:

```go
volume.ExBuildObjectStoreAPI(ctx, bus, false, objStoreID, StorageVolumeID(provID, accountID), cancel)
```

External code that needs to mount an ObjectStore must obtain the volume
reference from the appropriate provider account. Do not reconstruct the ID from
parts.

## Package Boundaries

### web/ And app/

`web/` is the plugin-importable component library. Put code in `web/` only if
plugins import it or reasonably would: UI primitives, hooks, SDK wrappers,
ObjectViewer framework, and reusable utilities such as `useForgeBlockData`.

`app/` is application-specific code: object type viewers, pages, session
management, shell components, window chrome, loading screens, and quickstart
flows.

- Plugins import from `@s4wave/web/` only.
- Plugins must never import from `@s4wave/app/`.
- `app/` may import from both `@s4wave/web/` and `@s4wave/app/`.
- Verify boundary violations with:

  ```bash
  rg "from '@s4wave/app/" web/
  ```

  Exclude `web/test/helpers.tsx` when evaluating results.

### Singleton Library Imports

Singleton library APIs must be imported through `web/` re-exports. The bldr
build produces separate bundles for `spacewave-app` and `spacewave-web`.
Libraries that rely on shared global singleton state can be duplicated across
bundles when imported directly from both.

Example: import `toast` from `@s4wave/web/ui/toaster.js`, not from `sonner`
directly.

### Viewer Registry

Object type viewers are registered statically in `app/viewers.tsx` and injected
into the `web/object/` ObjectViewer framework through `ViewerRegistryProvider`
from `web/hooks/useViewerRegistry.tsx`.

The app wraps its root with this provider. The framework reads viewers from
`useViewerRegistry()` so `web/` stays free of `app/` imports.

### Separate Plugins

Create a separate plugin under `plugin/` when a module has large dependencies
that would bloat the main bundle, such as Lexical or v86.

Merge lightweight viewers and services into `spacewave-app` with static
registrations in `app/viewers.tsx` and `sdk/`. The notes and VM plugins stay
separate.

## Frontend And UI Rules

### Session Routing

Components inside the session tree use React contexts instead of parsing URLs.

- Use `useSessionIndex()` from `web/contexts/` to get the session index.
- Use relative navigation such as `./free` and `../setup` for subtree-local
  moves.
- Use `useSessionNavigate()` for session-root navigation such as `join`,
  `so/${spaceId}`, and dashboard root.
- Do not reconstruct `/u/${sessionIndex}/...` strings or depend on nested
  `../..` path math.
- `SessionIndexContext` and `SessionRouteContext` are set by `AppSession`.
- Prefer context over URL parsing for other session-scoped state as well.

Session indexes start at 1. The `mountSessionByIdx` Resource SDK call uses
1-based indexes. In `AppSession.tsx`, `parseInt(param ?? '') || null` producing
`null` for index 0 is correct.

### Frontend Network And Crypto

TypeScript frontend code must use Go RPCs for crypto, HTTP, and WebSocket
operations.

- Do not implement cryptographic operations in TypeScript.
- Do not make direct cloud HTTP requests in TypeScript.
- Do not open raw WebSocket connections in TypeScript.
- Use the Go WASM runtime through in-process StarPC RPCs in the Resource SDK.
- If an RPC does not exist, add one to the proto and implement it in Go.

### Persisting UI State

Use `@s4wave/web/state/persist.tsx` for UI state that should survive reloads,
such as view modes, collapsed sections, and scroll positions.

```tsx
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'

function Viewer() {
	const gitNs = useStateNamespace(['git'])
	const [viewMode, setViewMode] = useStateAtom<'files' | 'readme' | 'log'>(
		gitNs,
		'viewMode',
		'files',
	)
}
```

`SpaceObjectContainer` provides a parent namespace
`['objectViewer', objectKey]`. Viewer components scope beneath it with one
domain prefix:

- `useStateNamespace(['git'])` produces
  `['objectViewer', objectKey, 'git']`.
- `useStateNamespace(['canvas'])` produces
  `['objectViewer', objectKey, 'canvas']`.

Do not include `objectKey` in the viewer namespace. Do not use an empty
namespace because it collides with other viewer state at the same level.

### Bottom Bar Registration

- `BottomBarLevel` props must be stable.
- Wrap `button` renderers in `useCallback`.
- Wrap `overlay` elements in `useMemo`.
- Pass `buttonKey` and `overlayKey` whenever rendered content should update.
- Keys should encode the data that drives the UI, such as selected names,
  object IDs, or open state.
- `overlay` is read lazily by the root. Update memoized data and bump the key so
  `SessionFrame` re-renders with fresh content.
- Do not return raw `resource_id` values or inline JSX that depends on stale
  closures inside `BottomBarLevel`.

### Image Imports

Use Vite static asset imports for images:

```tsx
import gridPattern from '../images/patterns/grid.png'
```

The imported value is a resolved URL string at build time. Do not use
`new URL(..., import.meta.url).href` for image assets.

### Icon Library

Prioritize React icon libraries in this order:

1. `react-icons/lu` (Lucide)
2. `react-icons/ri` (Remix Icon)
3. `react-icons/pi` (Phosphor)
4. `react-icons/rx` (Radix UI)

Use consistent icon families within related components. Use other icon families
only when these libraries lack a suitable icon. Prefer filled variants where
they match surrounding icons.

Common mappings:

- Chevrons: `LuChevronDown`, `LuChevronRight`, `LuChevronLeft`, `LuChevronUp`
- Arrows: `LuArrowLeft`, `LuArrowRight`, `LuArrowUp`, `LuArrowDown`
- UI actions: `LuSearch`, `LuPlus`, `LuMenu`, `LuX`, `LuCopy`
- Files: `LuFolder`, `LuFile`, `LuHome`, `LuHardDrive`
- Media: `LuPlay`, `LuPause`, `LuSkipForward`, `LuSkipBack`

### UX Heuristics

Keep these UX laws in mind when working on UI. If you encounter a violation and
the fix is outside the current task, flag it before changing scope.

Heuristics:

- Aesthetic-Usability Effect
- Fitt's Law
- Goal-Gradient Effect
- Hick's Law
- Jakob's Law
- Miller's Law
- Parkinson's Law

Principles:

- Doherty Threshold
- Occam's Razor
- Pareto Principle
- Postel's Law
- Tesler's Law

Gestalt:

- Law of Common Region
- Law of Proximity
- Law of Pragnanz
- Law of Similarity
- Law of Uniform Connectedness

Cognitive biases:

- Peak-End Rule
- Serial Position Effect
- Von Restorff Effect
- Zeigarnik Effect

## Proto And Generated Sources

### Proto Imports

Proto files use Go-style import paths based on Go module names from `go.mod`.
This module is `github.com/s4wave/spacewave`.

Local proto files reference each other with the full Go module path:

```protobuf
import "github.com/s4wave/spacewave/core/session/session.proto";
import "github.com/s4wave/spacewave/core/sobject/sobject.proto";
```

Package naming:

- `sdk/` proto files use the full `s4wave.` prefix, such as
  `package s4wave.space;`.
- `core/` proto files use shortened package names without the prefix, such as
  `package space.world;`.
- When `sdk/` files reference types from `core/` packages, use leading-dot fully
  qualified references such as `.space.world.WorldContents`.

### Dist Sources

When adding TypeScript files that need to be bundled for Electron or browser
entrypoints, add them to the `//go:embed` directives in `dist.go`.

`DistSources` contains TypeScript sources used by esbuild during the build. If a
new `.pb.ts` file or TypeScript module is imported by files in `web/electron/`
or `web/entrypoint/`, it must be explicitly embedded.

## Testing And Build Commands

### Preferred Test Commands

Run all tests with abbreviated output:

```bash
bun testcheck
```

This runs JS unit tests, browser E2E tests, and Go tests, showing only a summary
unless something fails.

For full verbose output:

```bash
bun run test
```

Use `bun run test` or `bun testcheck`, not `bun test`. `bun test` invokes Bun's
built-in test runner instead of package scripts.

### Linting And Typechecking

After code changes, verify with the relevant subset of:

```bash
bun run typecheck
bun run lint
go build ./...
```

### Rebuilding .bldr

If `.bldr` has stale exports or module resolution errors, rebuild it with:

```bash
bun run setup
```

### Testbed Over Mocks

Prefer the `testbed` package and real in-memory running versions of the stack
over mocks.

Use `testbed.Default(ctx)` for a fully wired bus with volume, logger, and static
resolver. Add real controller factories to the static resolver rather than
mocking interfaces.

### E2E WASM Tests

Never call `h.Navigate()` in `e2e/wasm/` tests. `Navigate` calls Playwright
`page.Goto()`, which triggers a full HTTP page reload and destroys the WASM
process, plugin workers, and WebSocket connections.

Use client-side routing that preserves the WASM process:

```go
page.Evaluate(`() => {
	window.history.pushState({}, '', '/target/route')
	window.dispatchEvent(new PopStateEvent('popstate'))
}`)
```

The `nonavigate` linter at `lint/nonavigate/` enforces this rule. Build the
custom linter binary with `golangci-lint custom` using `.custom-gcl.yml`.

Use `core/resource/testbed/testbed_e2e_test.go` as the example for adding an
end-to-end test of a Resource SDK implementation.

`e2e/wasm` suites are opt-in. Set `ENABLE_E2E_WASM=true` before running
`go test` against `e2e/wasm` packages. New `e2e/wasm` packages need the same
`TestMain` gate before booting the harness.

### Debugging E2E Test Timeouts

When `core/e2e` tests time out, the issue is often a TypeScript test failure
that does not propagate cleanly. Debug with:

```bash
cd core && timeout 35 go test -timeout=30s -v -run TestSpacewaveCoreE2E ./e2e/... 2>&1 | grep -E "panic|ERROR|test failed|test completed"
```

Common causes:

- Proto validation errors. TypeScript tests must populate required proto fields
  such as `timestamp`.
- Missing service implementations. Check whether an unimplemented RPC is being
  called.
- Directive imbalance. Compare added and removed directives to find stuck
  lookups.

TypeScript proto mapping:

- `google.protobuf.Timestamp` maps to `Date`; use `new Date()` to populate it.
