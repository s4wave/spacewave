# Spacewave Agent Guide

This file is the repository-local alignment layer for `repos/spacewave`.

This file assumes any broader workspace `AGENTS.md` that applies to the session
has already been read before this repository-local guide. When this is the only
guide available, read it with `README.md`, `DESIGN.md`, `package.json`,
`go.mod`, and the files near the target code before editing.

## Skill-Style Entry Contract

- Keep following the broader workspace rulebook when one is in force; this file
  only adds Spacewave-specific alignment.
- If the broader workspace provides private workflow, vocabulary, planning,
  design, or review authority, use those owners before substantial design or
  code instead of redefining them here.
- When this is the only guide in force, build a short read list from
  `README.md`, `DESIGN.md`, this file, nearby code, package scripts, tests, and
  the subsystem README when present.
- State the work surface and proof event before editing. For behavior-changing
  work, name the owner, invariant, falsifier, proof command, and adjacent risk.
- Ask before branch changes, commits, pushes, releases, live-service mutation,
  credentials, destructive cleanup, public API changes, durable data breaks, or
  behavior changes that materially affect lifecycle, concurrency, persistence,
  sync, latency, or user-visible behavior.
- Preserve concurrent work. Treat dirty files you did not edit as user/agent
  work; inspect before depending on them and never revert them unless asked.
- Keep private planning state, notes, phase numbers, and internal workflow
  references out of Spacewave code, comments, commits, and PRs.

## Product Model

Spacewave is a local-first Go and TypeScript application framework for
peer-to-peer collaborative apps on Hydra and Bifrost. The shared substrate is
Spaces, SharedObjects, storage/sync, Spacewave Cloud, PluginHost, and Bldr
plugin loading; focused apps and plugins are front doors over that substrate.

Runtime reach must not fork the product model:

- Go owns SDK/core semantics and native runtime paths.
- Browser Go plugins target `web/js/wasm` through Bldr/TinyGo and run in
  browser workers.
- TypeScript plugins target `js` through WebWorker, QuickJS, and browser
  QuickJS paths.
- GoScript is a selected-package Go-to-TypeScript compiler for browser builds
  and staging gates, not a guarantee that every Go package has GoScript parity.
- Every Bldr plugin has its own bus. Cross-plugin behavior uses explicit
  RPC/resource/plugin-host boundaries; do not assume directives cross buses.
- A frontend `entrypoint=True` JS plugin can still render its root WebView
  across separate `-core`/`-web`/`-app` plugins. Use the existing
  `-core`/`-web`/`-app` layout for app/plugin structure questions.

Spacewave is alpha with real users. Clean breaks are preferred inside the repo,
but never silently break durable local state, cloud state, database migrations,
wire/storage formats, or user-owned data. Surface the reset/migrate/compat
choice first.

## Owner-Level Defaults

- Fix the owner of the invariant, state machine, lifecycle, resource, cache,
  storage, wire format, or UI data flow. Do not add caller-side guards that
  reconstruct owner state.
- Prefer the simplest complete design, not the smallest diff. Delete
  speculative code and shallow helpers; keep necessary owner rewrites.
- No fixed-interval polling for state. Owners expose waits, watches,
  broadcasts, conditions, event streams, or context-aware blocking calls.
- No fire-and-forget goroutines from callbacks, handlers, WebSocket frames, or
  hot paths. Use `util/routine`, `util/keyed`, `util/refcount`, and
  `broadcast.Broadcast` under an owning lifecycle context.
- Do not add dead-code fallback paths for impossible conditions. If construction
  guarantees a field, enforce construction and let violations fail at the owner.
- Comments record durable invariants, contracts, boundaries, algorithms, or
  historical failures only. Do not narrate the code or the session.
- Docs describe the current system in present-state wording, not migration
  history, plan phases, or temporary implementation notes.

## Repository Basics

- Work from the repository root unless a command explicitly belongs in a
  subdirectory.
- Use `rg` for search, `bun` for JS/TS scripts, `uv` for Python, and repo-local
  commands with explicit working directories.
- Use `bun run ...`, not `npm`, `yarn`, `pnpm`, or bare package binaries.
- Do not assume `bldr setup` needs a manual run. Bldr runs setup automatically
  for most operations; run `bun run setup` only to repair stale `.bldr` exports
  or module resolution.
- Do not edit, copy into, or sync files under `.bldr/src/`; it is generated.
  Edit source files in their original locations.
- Keep large command output under `.tmp/` when needed, then inspect a short
  summary or tail. Do not commit `.tmp/`.
- Do not use sleep loops for readiness, locks, files, ports, pids, or results.
  Prefer one blocking command with a real timeout, or an event-backed background
  run whose completion is reported once.
- Do not add new prototypes in this repo unless explicitly asked and the target
  runtime cannot be exercised elsewhere. Production tests do not live in
  prototype directories.

## Git, Branches, And Releases

- Do not commit, push, or create/switch branches unless explicitly asked.
- Default branch is `master`. Feature branches use `feat/<name>`; fixes use
  `fix/<name>`.
- If asked to commit, use DCO sign-off and conventional lowercase imperative
  subjects, for example `fix(db): bound opfs root lookup`.
- Never mention private plans/scopes/phases or test-passing summaries in commit
  messages or PRs. Name the product/code change.
- Do not push to `release` unless explicitly asked. When asked to push ordinary
  work, push `master`. Fast-forwarding `release` to `master` is a separate
  explicit release action; no merge commits.
- Before any discard or restore, inspect `git status --short` and the exact
  path diff. Reverse only hunks you authored.

## Dependencies And Vendoring

- Avoid new dependencies for shallow helpers or adapter glue. Prefer stdlib,
  existing repo packages, generated codecs, and owner-local code.
- Before adding an external dependency, check maintenance, license, API shape,
  and why existing code is not the right owner. Ask before continuing if the
  dependency is stale, low-trust, or changes the public surface.
- Temporary local `replace` directives may use absolute paths for local proof,
  but must not be committed. Before landing, publish the replaced repo or use a
  real module version, then remove the local replace.
- After any `go.mod` change, run `go mod tidy && go mod vendor`.
- Never edit `vendor/` or the module cache directly. Patch dependencies in a
  fork or upstream, then update `go.mod` and regenerate vendor.

## Go Rules

- Read `style/principles.org` and `style/go.org` when available. If they are
  not available, derive the same rules from the local code: one owner per
  invariant, scannable file contracts, lifecycle-owned concurrency, earned
  abstractions.
- Package/type structure matters more than helper fragmentation. Inline helpers
  that only name local shape checks; extract helpers only for real invariants,
  lifecycle transitions, domain operations, or shared mechanics.
- `cmd/...` packages are adapters for flags, terminal formatting, process
  setup, and calls into domain packages. Durable state transitions, graph or
  storage mutations, runner scheduling, daemon policy, lifecycle recovery, and
  reusable validation belong in owner packages.
- One exported struct per `.go` file when practical, named after the struct.
  File layout: imports, exported type, constructor, getters, `Execute(ctx)` if
  present, other context methods, non-context work, private helpers,
  compile-time assertions.
- Imports have two groups: stdlib then external, one blank line between groups.
- Doc comments start with the identifier and end with a period.
- Prefer getters over cross-struct field chains.
- Use `gopls rename` for symbol renames; do not manual find/replace
  identifiers.
- Avoid `fmt`, stdlib `log`, package-global loggers, `encoding/json`,
  `reflect`, `sync.Map`, `interface{}` maps, and `panic` for control flow.
  Use `strconv`, `github.com/pkg/errors`, explicit `*logrus.Entry` plumbing,
  generated codecs, typed maps, and explicit owner APIs.
- Log through a plumbed `le *logrus.Entry`; logrus entry methods are nil-safe,
  so do not add defensive nil checks around them.
- Prefer `slices`/`maps` helpers over `sort` unless implementing
  `sort.Interface` or using a legacy API.
- Check new objects for `Release()` or `Close()` and bind cleanup immediately.
- Never discard directive refs with `_`; capture and release refs from
  ControllerBus calls.
- Go filenames use `-`, not `_`, except build tags and `_test.go`.
- Build throwaway binaries through repo scripts or an ignored path. Do not put
  generated binaries in source directories unless a repo script owns that path.

## TypeScript, React, And UI

- Follow `DESIGN.md` for visual language. Styling uses Tailwind v4 theme
  variables in `web/style/app.css`; do not assume utility pixel sizes such as
  `h-4` or `h-16`.
- Avoid CSS gradients unless an existing design pattern requires one.
- Use `cn()` for conditional classes; do not interpolate class strings.
- Import hooks directly (`useMemo`, not `React.useMemo`), prefer
  `useCallback`/`useMemo`, and merge related `useState` values or use a
  reducer.
- Function declarations for React components. One exported component per file,
  PascalCase filename, co-located tests as `ComponentName.test.tsx`.
- Import paths use `.js` suffixes, even when importing `.ts` or `.tsx` files.
- Import groups: React/external, internal aliases, local/relative, separated by
  blank lines.
- Use `using` declarations for fixed lexical SDK/resources. Cleanup stacks are
  only for dynamic resource sets.
- Raw `useEffect` plus `useState` is not the async data pattern. Use
  `useResource`, `useStreamingResource`, `useMappedResource`,
  `useWatchStateRpc`, `useGetValueRpc`, `useSetValueRpc`,
  `useRetryWithAbort`, `useAbortSignalEffect`, or local app hooks such as
  `useRootResource` and `useStateAtomResource`.
- Raw `useEffect` is only for DOM side effects that load no async data and call
  no RPCs.
- React Doctor suppressions are architecture smells. Split components, move
  async/state into hooks/resources, or use SDK/resource primitives before
  suppressing; suppress narrowly with the owner reason.
- Prefer icon libraries in this order: `react-icons/lu`, `react-icons/ri`,
  `react-icons/pi`, `react-icons/rx`. Keep related components on one icon
  family when possible.
- Use Vite static asset imports for images; do not use
  `new URL(..., import.meta.url).href`.

## Frontend State And Routing

- Components inside the session tree use React contexts, not global URL
  parsing. Use `useSessionIndex`, `usePath`, router context,
  `useSessionNavigate`, and relative navigation.
- Do not reconstruct `/u/${sessionIndex}/...` paths. `SessionIndexContext` and
  `SessionRouteContext` are set by `AppSession`; session indexes are 1-based.
- TypeScript frontend code must not implement crypto, make direct cloud HTTP
  requests, or open raw cloud WebSockets. Add Go RPCs through the Resource SDK
  when the UI needs crypto, HTTP, or WebSocket behavior.
- Persist reload-surviving UI state with `@s4wave/web/state/persist.tsx`.
  Viewer components scope under the provided `['objectViewer', objectKey]`
  namespace with one domain prefix; do not include `objectKey` again and do not
  use an empty namespace.
- `BottomBarLevel` props must be stable. Wrap button renderers in
  `useCallback`, overlay elements in `useMemo`, and pass keys when rendered
  content should update.

## Package Boundaries

- `web/` is the plugin-importable component and SDK surface: UI primitives,
  hooks, SDK wrappers, ObjectViewer framework, and reusable utilities.
- `app/` is Spacewave application code: viewers, pages, session management,
  shell, window chrome, loading screens, and quickstarts.
- Plugins import from `@s4wave/web/`, never `@s4wave/app/`. `app/` may import
  from both.
- When adding plugin-importable files under `web/`, update the nearest
  `index.ts` barrel so `spacewave-web` exposes the API.
- Singleton library APIs must be imported through `web/` re-exports when shared
  across bundles. Example: import `toast` from `@s4wave/web/ui/toaster.js`, not
  directly from `sonner`.
- Object viewers are registered in `app/viewers.tsx` and injected into
  `web/object/` through `ViewerRegistryProvider`.
- Create a separate plugin under `plugin/` when a module has large dependencies
  that would bloat the main bundle. Merge lightweight viewers/services into
  `spacewave-app` with static registrations.

## Bldr Build And Runtime

- Bldr setup output is generated. Do not edit `.bldr/src/` or hand-copy files
  there.
- Controller factories are normally registered through `bldr.yaml` `configSet`
  entries and package scans, not production `AddFactory` calls. Direct
  `AddFactory` belongs in tests.
- `bldr/util/gocompiler` owns platform signing hooks. Preserve the env-var
  contract for macOS and Windows signing, and keep signing a no-op when signing
  credentials are unset.
- `bldr/util/logfile` owns file logging. Preserve `--log-file`,
  `BLDR_LOG_FILE`, console-vs-file level separation, auto-default logs for dev
  and distribution entrypoints, and retention pruning.
- Bldr `webPkgs` in `bldr.yaml` are shared at runtime through `/b/pkg/...`.
  With `exclude: true`, another plugin must provide the package or runtime URLs
  will 404.
- When adding TypeScript that must be bundled for Electron or browser
  entrypoints, update the relevant `//go:embed` `DistSources` in `dist.go`.

## TinyGo, WASM, And Browser APIs

- Treat `syscall/js.Value.Call`, `Invoke`, `New`, `String`, and `js.FuncOf` as
  integration boundaries, not utilities.
- Keep browser API orchestration JS-owned: method dispatch, Promise chaining,
  object construction, typed-array allocation, Web Locks, MessagePort handling,
  and JS exception classification.
- Go should pass primitives or existing JS values and transfer bytes with
  `js.CopyBytesToJS` / `js.CopyBytesToGo`.
- Never block inside a JavaScript-to-Go callback. Copy or retain the needed
  values, start owner-lifecycle Go work, return to JS immediately, and report
  completion through the owning Promise, callback, stream, or port.
- Do not pass raw wasm-memory pointer/length pairs to helper JS for long-lived
  or large data.
- Bound heap pressure before crossing browser APIs; stream or chunk large
  uploads, block shards, SSTables, and RPC packets at the storage/transport
  owner.
- If Chrome reports `RuntimeError: unreachable`, DataView bounds failures, or
  `syscall/js.value*` frames, trace the exact `syscall/js` boundary first.
- Browser/WASM verification needs a real browser path when production crosses
  OPFS, ResourceAttach, Web Locks, MessagePort, blockshard, or runtime streams.

## RPC, Resources, Watches, And Cloud State

- SDK RPCs returning mutable state are server-streaming `Watch*` RPCs, not
  unary `Get*` RPCs. Unary RPCs are for immutable values or one-shot actions.
- The UI uses `useStreamingResource` or watch hooks for server-pushed state.
  Any state changed by CLI, another tab, or a background process must be
  reactive.
- Proto3 bool fields can deserialize to `undefined` in TypeScript. Use
  `field ?? false` or `!!field`; loading state checks the containing message
  for `null`.
- RPCs returning `resource_id` allocate server resources. Wrap IDs with
  `resourceRef.createRef(id)`, bind fixed lifetimes with `using`, and release
  refs. Never discard resource IDs with `void`.
- Do not add `Unregister*`/`Remove*` RPCs for resource lifecycle that is already
  owned by resource release.
- `useResource(...)` retries on `server-released` resources by default. Disable
  only when server release is expected and terminal; composite returns must
  expose resource IDs through `getResourceIds`.
- Mutable cloud-backed UI state follows:

  ```text
  UI -> SRPC/watch -> Go cache/tracker state -> cloud sync machinery
  ```

- Account state is fetched once when the session mounts, cached in the Go
  provider/ObjectStore, served to UI by Go Watch loops, and invalidated by hash
  changes or Session DO WebSocket notifications. Do not trigger redundant cloud
  HTTP requests from React render paths.
- Multiple React subscribers to the same Watch stream share one Go-side stream.
- Own mutable watches at route/container boundaries and pass snapshots through
  context. Do not start separate leaf-component watches for the same state.
- Do not attach shared mutable/persistent state to per-client Resource wrappers.
  Shared state lives on stable domain owners or registries keyed by stable
  identity; wrappers forward to those owners.

## Cloud HTTP And Wire Formats

- All HTTP traffic to `repos/spacewave-cloud` goes through
  `core/provider/spacewave/client.go`.
- Cloud API request and response bodies use proto-binary
  `MarshalVT`/`UnmarshalVT` with `Content-Type: application/octet-stream`.
  Every endpoint has typed request/response protos, including empty acks.
- Approved helpers include `doPostBinary`, `doGetBinary`, `doDelete`,
  `doPostStream`, and `doMultiSig`. Do not resurrect JSON helpers.
- Streaming bulk routes, such as packfiles, release artifacts, and R2
  passthrough, carry raw byte streams. Consume them with streaming copies, not
  full buffering into proto decode.
- Forbidden on the Spacewave <-> Cloud boundary: `doPostJSON`, `fastjson`,
  proto JSON methods, hand-rolled JSON bodies, and hand-parsed JSON responses.
- WebSocket frames from Cloud are binary envelope protos with oneof bodies.
  Parse with `UnmarshalVT` and switch on the oneof.
- In Go HTTP clients, drain unread response bodies to `io.Discard` before
  closing unless the full body has already been read or streamed to EOF.

## Data Model And IDs

- SharedObject IDs are lowercase ULIDs. A SharedObject-backed block store ID is
  the SharedObject ULID verbatim.
- Use `SobjectBlockStoreID(soID)` for clarity, but do not prefix or translate
  SharedObject IDs. Cloud `bstoreId` URL params equal `soID`.
- Other ULID-keyed resources also store the ULID verbatim; use separate typed
  columns/wrappers for disambiguation.
- When calling `volume.ExBuildObjectStoreAPI`, pass the mounted volume's
  `vol.GetID()`, never a reconstructed `StorageVolumeID(...)`. Plugin-host
  proxy volumes change IDs; reconstructing raw IDs can hang alias matching.
- When identifiers share a wire shape but mean different domain roles, encode
  the distinction in the owner library API with semantic helpers. Callers choose
  by meaning, not byte shape.

## Protobufs And Generated Sources

- Proto imports use Go module paths from `go.mod`, for example
  `github.com/s4wave/spacewave/core/session/session.proto`.
- `sdk/` proto packages use the full `s4wave.` prefix. `core/` protos use
  shorter package names. Cross-package type references use leading-dot fully
  qualified names.
- After changing `.proto`, stage the `.proto` files first if using `aptre`,
  then run `bun run gen`. Use `bun run gen:force` only when a forced rebuild is
  actually needed.
- Never hand-edit generated `*.pb.go`, `*.pb.ts`, `*.pb.gs.ts`, `*.pb.cc`, or
  `*.pb.h`; change the source proto and regenerate.
- Follow existing SDK/resource naming before inventing shapes. Use
  `sdk/world/world.proto` and `sdk/world/` as the reference for resource
  service naming, `MethodRequest`/`MethodResponse`, resource-id access, Go SDK
  wrappers, TS Resource wrappers, and generated exports.
- Block-backed state should be a clean block DAG under the owning World object.
  Make a separate World object only for independent identity, graph
  relationships, permissions, lifecycle, or cross-owner references.
- Block DAG field comments state key encoding and value type.
- Parse stable external payloads into typed proto fields. Raw JSON/wire payloads
  are optional debugging evidence, not the primary model.
- Reuse common proto enums and owner types. Do not duplicate generated enum
  values as handwritten constants.
- Add RPCs to the existing resource service when they operate on that service's
  state or security boundary. New services/packages need independent lifecycle
  or domain identity.
- Never hand-roll protobuf wire parsing or proto JSON; use generated codecs.

## Storage, Concurrency, And Controllers

- `BeginReadOperation`, `NewTransaction(false)`, bucket cursors, GC wrappers,
  projection hydration, and resource read scopes are lock-owning transaction
  boundaries.
- Never open a second read transaction on the same bbolt/kvtx-backed store
  while holding the first. During mmap growth this can deadlock the process.
- When layering wrappers, wrap the already-scoped store or add an owner API for
  the scoped wrapper; do not call an ordinary `BeginReadOperation` while already
  holding the underlying read scope.
- `broadcast.Broadcast` is the preferred shared-state mutex plus wait channel;
  name the field `bcast`. Read state and obtain the wait channel inside one
  `HoldLock`.
- Preferred util primitives:
  - shared state and waiters: `broadcast.Broadcast`
  - one goroutine per dynamic key: `keyed.Keyed`
  - ref-counted goroutine per key: `keyed.KeyedRefCount`
  - shared background resource: `refcount.RefCount`
  - single retrying goroutine: `routine.RoutineContainer`
  - state-gated goroutine: `routine.StateRoutineContainer`
  - watchable current value: `ccontainer.CContainer` plus broadcast
- The `ctx` passed to a controller `Execute()` is the controller lifecycle
  context. If the only work is wiring lifecycle-gated subsystems, hand them
  `ctx` and return nil; do not block just to keep `Execute()` alive.
- Do not `defer cancel()` or `defer ClearContext()` in an `Execute()` that
  returns after wiring subsystems. Cleanup tied to removal belongs in `Close()`.
- Do not store `context.Context` as a controller field unless it is a documented
  long-lived transport lifecycle context.
- Returning an error from `Execute()` restarts `Execute()` with backoff; it does
  not reconstruct the controller.

## Directive Patterns

- Name resolvers descriptively, store the directive on the resolver, use
  pointer receivers, and return resolver pointers.
- Use `directive.NewValueResolver` for static values,
  `directive.NewFuncResolver` for simple async/watch logic, and custom resolver
  types only for complex state.
- Watch loop shape: `ClearValues`; snapshot via `HoldLock(getWaitCh)`;
  compute/emit outside the lock; `MarkIdle(true)`; select on `waitCh` or
  `ctx.Done()`.
- In `HandleDirective`, do not filter on ephemeral state.
- Never call `AddValue` inside `bcast.HoldLock`; it can deadlock.
- Each directive gets an `Ex{DirectiveName}` helper wrapping
  `bus.ExecWaitValue`. Prefer `Ex` helpers over direct `ExecWaitValue`.

## Testing And Verification

Choose the narrowest tier that proves the changed behavior:

- JS unit tests: `*.test.ts` with Vitest/happy-dom for pure logic.
- Browser tests: `*.browser.test.ts` / `*.e2e.test.ts` in vitest browser mode
  for real browser APIs such as OPFS, BroadcastChannel, Web Locks, SharedArray
  Buffer, and workers.
- Playwright app E2E: use the relevant package script in `package.json` for
  full app lifecycle, WASM boot, plugin rendering, and console checks
  (`test:browser`, `test:go:e2e:*`, or a release lane).
- Release web E2E: `bun run test:release:web` for static release output.
- Go tests: `go test ./...`, scoped to the package when the change is narrow.

Common commands:

```bash
bun testcheck
bun run test
bun run check
bun run typecheck
bun run lint
go build ./...
go test ./...
```

Use `bun run test` or `bun testcheck`, not `bun test`; bare `bun test` invokes
Bun's built-in runner instead of package scripts.

Prefer `testbed.Default(ctx)` and real in-memory stack components over mocks.
Mock only the explicit boundary under test.

E2E WASM rules:

- Never call `h.Navigate()` in `e2e/wasm`; it reloads the page and destroys the
  WASM process, workers, and WebSockets. Use client-side history routing.
- `e2e/wasm` suites are opt-in with `ENABLE_E2E_WASM=true`. New packages need
  the same `TestMain` gate before booting the harness.
- Use `core/resource/testbed/testbed_e2e_test.go` as the reference for Resource
  SDK end-to-end tests.

After docs-only edits, run at least `git diff --check -- <files>`. After code
edits, run the focused tests plus formatting/lint/typecheck/build commands that
cover the touched owner.

## Public Docs And Links

- Public GitHub docs must link only to public repositories and public docs.
- Do not leak local absolute paths, private planning files, or internal
  plan/scope/review state into public docs, code comments, commits, or PRs.
- Keep this guide general-purpose. It captures repository rules, recurring
  patterns, and architectural invariants, not task-specific plans or historical
  work notes.
