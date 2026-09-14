# Spacewave Agent Guide

Spacewave is a local-first Go and TypeScript application framework for peer-to-peer collaborative apps. Read `README.md` for setup, `DESIGN.md` for the interface, and `package.json` and `bldr.star` for executable build and test commands.

## Browser Runtime: GoScript

Browser Go code uses GoScript, the Go-to-TypeScript compiler, and runs as JavaScript in browser workers. Use the GoScript build and test lanes for browser development. Native applications continue to use native Go.

WebAssembly is excluded from the current browser runtime direction, including as an optimization or compiler workaround. Our Go/WASM toolchain and browser integration have too many limitations: memory cannot resize as the application needs, binaries are too large, builds take too long, I/O is too slow, and the integration lacks native WASM GC and asynchronous WASM I/O. These limitations apply to the stack we can use today.

GoScript supports selected Go packages; check the actual compiled path when adding a dependency. A GoScript lowering, runtime, or typecheck defect belongs in `github.com/s4wave/goscript`, with a compliance fixture and an updated dependency here. Keep Spacewave source idiomatic Go and regenerate compiler output after fixing the compiler.

Some browser harness paths and environment variables retain `wasm` in their names. Select their GoScript mode explicitly; the directory name does not select the runtime.

## Product And Package Boundaries

Spaces, SharedObjects, storage, sync, cloud providers, the Resource SDK, and PluginHost form the shared application substrate. Apps and plugins use these components for account, data, and lifecycle behavior.

- `core/` implements application and resource services; `sdk/` exposes their Go and TypeScript clients.
- `db/` implements storage and World data; `net/` implements networking.
- `bldr/` builds, distributes, and loads plugins.
- `web/` is the plugin-importable UI and SDK surface: components, hooks, wrappers, and the ObjectViewer framework.
- `app/` contains the application shell, pages, viewers, sessions, and quickstarts.
- Plugins import from `@s4wave/web/`. Application code may also import from `@s4wave/app/`.
- Export new plugin-facing `web/` APIs through the nearest `index.ts` barrel. Shared singleton libraries also go through these exports, such as `toast` from `@s4wave/web/ui/toaster.js`.
- Register application viewers in `app/viewers.tsx`; `ViewerRegistryProvider` supplies them to `web/object/`.

Use the existing `-core`/`-web`/`-app` plugin layout. A frontend `entrypoint=True` plugin can render its root WebView across those plugins. Each Bldr plugin has its own ControllerBus; cross-plugin operations use explicit RPC, resource, and plugin-host interfaces.

## Setup And Commands

Run package scripts with `bun run` from the repository root. `package.json` owns test tags, timeouts, and opt-in environment variables.

- `bun install` installs dependencies, vendors Go modules, and runs setup. Let it finish before building or checking; concurrent vendoring replaces `vendor/` while readers are using it.
- `uv sync --frozen --all-groups` installs Python generation tools, including `protoc-gen-starpc-python` in `.venv`.
- `bun run setup` repairs Bldr exports and module resolution after dependencies are installed.
- After changing Go dependencies, run `go mod tidy` and `go mod vendor` sequentially. Dependency fixes belong in their source repositories; regenerate vendored output here.
- `bun run typecheck` checks TypeScript; `bun run check` adds lint.
- `bun run test` runs the standard JavaScript, browser, and Go suites. `bun run test:go` runs Go tests with the repository's exclusions and timeout.
- `go test ./path/to/pkg` is suitable for a focused Go package check.
- `bun run test:go:e2e:wasm:goscript` runs the GoScript browser harness.
- `bun run test:go:e2e:release-wasm:goscript` runs the GoScript release harness.
- `bun run test:release:web` exercises static release output.

Browser API tests use `*.browser.test.ts` or `*.e2e.test.ts` in Vitest browser mode. Full application tests use the relevant Playwright or Go E2E package script. In `e2e/wasm`, use client-side routing to retain the running process; `h.Navigate()` reloads the page and destroys its workers and WebSockets.

Look for existing testbeds before constructing mocks: `testbed/`, `db/testbed/`, `db/world/testbed/`, `db/unixfs/world/testbed/`, `core/resource/testbed/`, `core/resource/layout/testbed/`, `bldr/testbed/`, `net/testbed/`, `forge/testbed/`, and `sdk/testbed/`. Prefer `testbed.Default(ctx)` and real in-memory components.

The default branch is `master`. The `release` branch is a separate publication target; advancing it requires an explicit release instruction.

## Dependency Tooling

`.tools/` is a generated Go module for linters and generators. Its module files and `deps.go` come from the embedded tools module in `github.com/aperturerobotics/common`. `bun install` removes stale copies; `aptre` recreates them. Fix missing linter dependency hashes in that upstream tools module, run its `bash embed.bash`, and update the dependency here.

An npm package's `latest` tag can point to an older release line. Check the resolved version during dependency refreshes. Use `resolutions` and `overrides` in `package.json` when a required version is published under another tag.

## Bldr Builds And Controller Registration

Edit original source files; `.bldr/src/` and setup exports are generated.

The `goPkgs` field in a `bldr.star` manifest determines which Go controller factories Bldr bundles. A non-core controller omitted there cannot satisfy runtime `LoadController` or `LoadFactoryByConfig` directives. `configSet` creates startup instances; runtime factory lookup needs `goPkgs` alone. Use manifest registration for production factories and direct `AddFactory` in tests.

`FetchManifest` idle readbacks can precede a manifest's arrival. Trace the producing builder, selected platform IDs, directive references, and resolver state before changing build ordering.

Bldr `webPkgs` are shared through `/b/pkg/...`. A package marked `exclude: true` must be supplied by another plugin.

`DistSources` embeds TypeScript needed by browser, Electron, and downstream Bldr builds. Update the relevant `dist.go` when adding an imported source path. Include its transitive imports, using exact files or narrow extension globs. Downstream imports remain within the `web/` surface. Go dependencies arrive through `vendor/`; embedded `deps_only` stubs are for resolving proto packages. The repository-root `dist.go` owns root `web/` paths because `bldr/dist.go` can embed only descendants of `bldr/`.

`bldr/util/gocompiler` owns platform signing and its environment variables; signing is a no-op without credentials. `bldr/util/logfile` owns `--log-file`, `BLDR_LOG_FILE`, console and file levels, default log locations, and retention.

## React, Resources, And Routing

Use `DESIGN.md` and the Tailwind v4 theme in `web/style/app.css`. Theme utilities can differ from standard Tailwind pixel sizes. Use `cn()` for conditional classes and Vite imports for static images. Prefer icons from `react-icons/lu`, then `ri`, `pi`, and `rx`, keeping related components in one family.

Async UI data uses the existing resource hooks: `useResource`, `useStreamingResource`, `useMappedResource`, `useWatchStateRpc`, `useGetValueRpc`, `useSetValueRpc`, `useRetryWithAbort`, and the local session/app hooks. Reserve raw `useEffect` for DOM effects that load no async data and call no RPCs.

- Inside a session, use `useSessionIndex`, `usePath`, router context, `useSessionNavigate`, and relative navigation. `AppSession` supplies session contexts; session indexes are 1-based.
- Crypto and cloud HTTP/WebSocket operations run through Go services exposed by the Resource SDK.
- Persist UI state with `@s4wave/web/state/persist.tsx`. Viewers use the supplied `['objectViewer', objectKey]` namespace plus one domain prefix.
- Keep `BottomBarLevel` callbacks and overlay elements stable with `useCallback` and `useMemo`; use keys when rendered content should update.
- For fixed lexical SDK resource lifetimes, use `using`. Dynamic resource sets may use cleanup stacks.

## RPC, Watches, And Cloud

Expose changing state through server-streaming `Watch*` RPCs. Unary calls serve immutable values or one-shot actions. Route containers share watch snapshots through context; multiple subscribers to the same state share the Go-side stream.

RPCs returning `resource_id` allocate server resources. Wrap them with `resourceRef.createRef(id)` and release the resulting reference. Resource release owns teardown. Composite `useResource` values expose their IDs through `getResourceIds`; the hook retries server-released resources unless release is expected and terminal.

Go callers own each returned resource handle. Release every acquired `world.ObjectState` with `world.ReleaseObjectState`, and release resource references, cursors, and iterators through their existing cleanup API. Never discard a handle with `_`, including in existence checks or helpers returning only a body or error. Release it on success and on error paths after acquisition, or explicitly transfer ownership to the caller. Committing or discarding a transaction does not release its independently adopted remote object handles; they otherwise remain on the server until the client disconnects. Use the real RPC testbed for lifecycle regressions and verify resource counts return to baseline while the connection stays open.

Mutable shared state belongs on stable domain components or registries, while per-client Resource wrappers forward operations. Cloud-backed state flows from cloud sync into Go provider/ObjectStore caches, through watches, and into React. Hash changes and session WebSocket notifications invalidate the caches.

Proto3 bools can deserialize as `undefined` in TypeScript. Normalize with `field ?? false` or `!!field`; test the containing message for `null` to identify loading.

All Spacewave Cloud HTTP traffic uses `core/provider/spacewave/client.go`. Typed API bodies use generated proto-binary codecs and `Content-Type: application/octet-stream`, including empty acknowledgements. Use `doPostBinary`, `doGetBinary`, `doDelete`, `doPostStream`, or `doMultiSig` as appropriate. Bulk routes carry raw streams. Cloud WebSocket frames are binary envelope protos with oneof bodies.

## Data And Generated Sources

SharedObject IDs are lowercase ULIDs. A SharedObject block-store ID is that ULID verbatim; `SobjectBlockStoreID(soID)` expresses the relationship, and cloud `bstoreId` parameters use `soID`.

When calling `volume.ExBuildObjectStoreAPI`, pass `vol.GetID()` from the mounted volume. Plugin-host proxy volumes can change IDs; reconstructing the underlying ID can hang alias matching.

World object keys have the form `<stable-type-root>/<self-contained-id>`. Use the object's own opaque or natural identity. Cross-object relationships belong in graph edges, with a key-valued field only when a consumer needs that direct reference. Parent-scoped keys may describe bounded owned children. Each key builder has a parser that understands its own grammar. Durable key changes require migration; an authorized rename uses `RenameObject(descendants=true)` and updates graph quads containing the key.

Block-backed state forms a block DAG under its World object. Create a separate World object for independent identity, permissions, lifecycle, or graph relationships. Block DAG comments specify key encoding and value type.

- Proto imports use Go module paths from `go.mod`.
- `sdk/` proto packages use the full `s4wave.` prefix; `core/` packages use shorter names. Cross-package type references are fully qualified with a leading dot.
- Use `sdk/world/world.proto` and `sdk/world/` as references for resource services, request/response names, resource IDs, and SDK wrappers.
- When using `aptre`, stage changed `.proto` files before `bun run gen`. Regenerate all affected sources; use `bun run gen:force` only for a required forced rebuild.
- Reuse generated codecs, enums, and domain types. Parse stable external payloads into typed fields; raw payloads may accompany them as debugging evidence.

## Storage And Controller Lifetimes

A Space can hold unbounded data. Opening a store, volume, World, or index must not scan or retain memory proportional to its contents. Read through stored indexes so work scales with the answer. Measure opening a durable store at meaningful sizes when changing this behavior.

`BeginReadOperation`, `NewTransaction(false)`, bucket cursors, GC wrappers, projection hydration, and resource read scopes can hold transaction locks. Reuse the existing scope when layering stores. A second read transaction on the same bbolt/kvtx store while the first remains open can deadlock during mmap growth.

`broadcast.Broadcast` combines shared state with notifications. Read the state and obtain its wait channel inside one `HoldLock`; emit directive values outside the lock.

A controller's `Execute(ctx)` receives its lifecycle context. It can wire subsystems to that context and return nil. Cleanup on controller removal belongs in `Close()`, so a returning `Execute()` must not defer cancellation of those subsystems. An error restarts `Execute()` with backoff on the same controller instance.

Resolvers use `directive.NewValueResolver` for static values and `directive.NewFuncResolver` for simple asynchronous resolution. A watch clears values, snapshots state and its wait channel, emits outside the lock, marks idle, then waits for notification or cancellation. `HandleDirective` matches the directive independently of transient state.

Use existing `Ex*` and `loader.WaitExecControllerRunning*` helpers. Keep any returned live `directive.Reference` until its caller's resource or lifecycle releases it.

## Documentation Audiences

Each page in `app/docs/content/` belongs to one audience:

- `users/`: explain what to click and what happens, using terms visible in the app. Drive, Space, and `getting-started.md` belong here. Internal names such as SharedObject, World, UnixFS, and provider IDs belong in developer documentation. CLI commands, flags, paths, and environment variables stay exact because users type them.
- `self-hosters/`: explain operating disks, backups, servers, and recovery using the product's operational names.
- `developers/`: explain APIs, packages, protocols, and internal mechanisms with exact technical names.
