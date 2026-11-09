# Spacewave agent guide

Spacewave is a local-first Go and TypeScript framework for peer-to-peer collaborative apps. Read `README.md` for setup, `DESIGN.md` for the interface, and `package.json` and `bldr.star` for the build and test commands.

## Browser runtime: GoScript

Browser Go code is compiled to TypeScript by GoScript and runs as JavaScript in browser workers. Use the GoScript build and test lanes for browser work. Native applications use native Go.

The browser runtime does not use WebAssembly, including as an optimization or a compiler workaround. The Go WebAssembly toolchain and browser integration we can use today have too many limits: memory cannot grow as the application needs, binaries are too large, builds are too slow, I/O is too slow, and there is no native WebAssembly GC or asynchronous I/O.

GoScript supports a subset of Go packages, so check the compiled path when you add a dependency. Fix a GoScript lowering, runtime or typecheck defect in `github.com/s4wave/goscript` with a compliance fixture, then update the dependency here. Keep Spacewave source idiomatic Go and regenerate the compiler output after the compiler fix.

Some browser harness paths and environment variables still contain `wasm`. Select GoScript mode explicitly; the directory name does not choose the runtime.

## Products and packages

Spaces, SharedObjects, storage, sync, cloud providers, the Resource SDK and PluginHost form the shared application substrate. Apps and plugins use them for accounts, data and lifecycle.

- `core/` implements application and resource services; `sdk/` holds their Go and TypeScript clients.
- `db/` implements storage and World data; `net/` implements networking.
- `bldr/` builds, distributes and loads plugins.
- `web/` is the UI and SDK surface plugins import: components, hooks, wrappers and the ObjectViewer framework.
- `app/` holds the application shell, pages, viewers, sessions and quickstarts.
- Plugins import from `@s4wave/web/`. Application code may also import from `@s4wave/app/`.
- Export each new plugin-facing `web/` API through the nearest `index.ts` barrel, including shared singleton libraries such as `toast` from `@s4wave/web/ui/toaster.js`.
- Register application viewers in `app/viewers.tsx`; `ViewerRegistryProvider` passes them to `web/object/`.

Use the existing `-core`, `-web` and `-app` plugin layout. A frontend plugin with `entrypoint=True` can render its root WebView across those plugins. Each Bldr plugin has its own ControllerBus, so cross-plugin calls go through explicit RPC, resource and plugin-host interfaces.

## Setup and commands

Run package scripts with `bun run` from the repository root. `package.json` defines test tags, timeouts and opt-in environment variables.

- `bun install` installs dependencies, vendors Go modules and runs setup. Let it finish before building or checking: vendoring replaces `vendor/` while other commands read it.
- `uv sync --frozen --all-groups` installs the Python generation tools, including `protoc-gen-starpc-python` in `.venv`.
- `bun run setup` repairs Bldr exports and module resolution after an install.
- After changing Go dependencies, run `go mod tidy`, then `go mod vendor`. Fix a dependency in its own repository and regenerate the vendored copy here.
- `bun run typecheck` checks TypeScript; `bun run check` also lints.
- `bun run test` runs the standard JavaScript, browser and Go suites. `bun run test:go` runs the Go tests with the repository's exclusions and timeout.
- `go test ./path/to/pkg` checks one Go package.
- `bun run test:go:e2e:wasm:goscript` runs the GoScript browser harness.
- `bun run test:go:e2e:release-wasm:goscript` runs the GoScript release harness.
- `bun run test:release:web` exercises the static release output.

Browser API tests are `*.browser.test.ts` or `*.e2e.test.ts` files in Vitest browser mode. Full application tests use the matching Playwright or Go end-to-end package script. In `e2e/wasm`, navigate with client-side routing to keep the running process; `h.Navigate()` reloads the page and destroys its workers and WebSockets.

Look for an existing testbed before writing a mock: `testbed/`, `db/testbed/`, `db/world/testbed/`, `db/unixfs/world/testbed/`, `core/resource/testbed/`, `core/resource/layout/testbed/`, `bldr/testbed/`, `net/testbed/`, `forge/testbed/` and `sdk/testbed/`. Prefer `testbed.Default(ctx)` and real in-memory components.

The default branch is `master`. The `release` branch is a separate publication target; advance it only on an explicit release instruction.

## Go code paragraphs

Write every Go function body with more than one action, including tests, as a sequence of code paragraphs. A paragraph is one action: several statements, or a control structure with its error return. Put a purpose comment directly above each paragraph and one blank line after it, so the comments read in order as an outline of the function. A comment describes only the paragraph below it and refers to the concrete component or record; it never restates a single statement. A `defer` that releases what the paragraph acquired stays in that paragraph.

A function with one action, such as a direct return, a delegated call, a small adapter or a constructor, stays one paragraph under its declaration comment.

```go
// Start a testbed whose block store holds the encrypted World.
tb, err := testbed.NewTestbed(ctx, le)
if err != nil {
	t.Fatal(err)
}
defer tb.Release()

// Encrypt every block with an inline transform the snapshot can carry.
transform, err := block_transform.NewConfig(confs)
if err != nil {
	t.Fatal(err)
}
```

Braces and `if err != nil` blocks do not end a paragraph. When you edit a Go file, bring each function you touch to this shape. The pre-commit hook `.githooks/go-paragraphs` rejects a staged function that has a changed line and breaks the rule, so fix it in the same commit. Comment every top-level declaration with a sentence that starts with its identifier.

## Dependency tooling

`.tools/` is a generated Go module for linters and generators. Its module files and `deps.go` come from the tools module embedded in `github.com/aperturerobotics/common`. `bun install` removes stale copies and `aptre` recreates them. Fix a missing linter dependency hash in that upstream tools module, run its `bash embed.bash`, and update the dependency here.

An npm package's `latest` tag can point to an older release line, so check the resolved version when refreshing dependencies. When a required version is published under another tag, pin it with `resolutions` and `overrides` in `package.json`.

## Bldr builds and controller registration

Edit the original source files; `.bldr/src/` and the setup exports are generated.

A `bldr.star` manifest's `goPkgs` field decides which Go controller factories Bldr bundles. A non-core controller missing from that list cannot satisfy a runtime `LoadController` or `LoadFactoryByConfig` directive. `configSet` creates startup instances; runtime factory lookup needs only `goPkgs`. Register production factories in the manifest and call `AddFactory` directly in tests.

An idle `FetchManifest` readback can come before the manifest arrives. Trace the producing builder, the selected platform IDs, the directive references and the resolver state before you change build ordering.

Bldr shares `webPkgs` through `/b/pkg/...`. Another plugin must supply a package marked `exclude: true`.

`DistSources` embeds the TypeScript that browser, Electron and downstream Bldr builds import. When you add, rename or delete an imported source path, update the matching `dist.go` with the path and its transitive imports, as exact files or narrow extension globs, and run `go test ./web/ ./bldr/`: a stale embed breaks the Go build, and `TestDistSourcesAreClosed` catches a missing import. Downstream imports stay within the `web/` surface. Go dependencies arrive through `vendor/`; the embedded `deps_only` stubs only resolve proto packages. The repository-root `dist.go` covers root `web/` paths, because `bldr/dist.go` can embed only paths under `bldr/`.

`bldr/util/gocompiler` implements platform signing and reads its environment variables; without credentials, signing does nothing. `bldr/util/logfile` implements `--log-file`, `BLDR_LOG_FILE`, the console and file levels, the default log locations and retention.

## React, resources and routing

Follow `DESIGN.md` and the Tailwind v4 theme in `web/style/app.css`; theme utilities can differ from standard Tailwind pixel sizes. Use `cn()` for conditional classes and Vite imports for static images. Take icons from `react-icons/lu` first, then `ri`, `pi` and `rx`, keeping related components in one family.

Load async UI data through the existing resource hooks: `useResource`, `useStreamingResource`, `useMappedResource`, `useWatchStateRpc`, `useGetValueRpc`, `useSetValueRpc`, `useRetryWithAbort` and the local session and app hooks. Use a raw `useEffect` only for a DOM effect that loads no async data and calls no RPC.

- Inside a session, use `useSessionIndex`, `usePath`, the router context, `useSessionNavigate` and relative navigation. `AppSession` provides the session contexts; session indexes start at 1.
- Crypto and cloud HTTP and WebSocket operations run in Go services that the Resource SDK exposes.
- Persist UI state with `@s4wave/web/state/persist.tsx`. A viewer uses its `['objectViewer', objectKey]` namespace plus one domain prefix.
- Keep `BottomBarLevel` callbacks and overlay elements stable with `useCallback` and `useMemo`; set a key when rendered content should update.
- Use `using` for an SDK resource with a fixed lexical lifetime. A dynamic set of resources may use a cleanup stack.

## RPC, watches and cloud

Expose changing state through server-streaming `Watch*` RPCs. Unary calls return immutable values or perform one-shot actions. Route containers share watch snapshots through context. An SDK watch that takes a key, such as a `Root` or `Session` watch or a keyed `useWorldQuery`, shares one Go-side stream among its subscribers with `SharedStreams`; an unkeyed watch opens its own stream.

An RPC that returns a `resource_id` allocates a server resource. Wrap it with `resourceRef.createRef(id)` and release the reference; releasing the resource tears it down. A composite `useResource` value exposes its IDs through `getResourceIds`, and the hook retries a resource the server released unless that release is expected and terminal.

A Go caller is responsible for each resource handle it receives. Release every acquired `world.ObjectState` with `world.ReleaseObjectState`, and release resource references, cursors and iterators through their cleanup API. Never discard a handle with `_`, including in existence checks and in helpers that return only a body or an error. Release the handle on success and on every error path after acquiring it, or hand it to the caller explicitly. Committing or discarding a transaction does not release the remote object handles it adopted; they stay on the server until the client disconnects. Test lifecycle regressions on the real RPC testbed and check that resource counts return to baseline while the connection stays open.

Keep mutable shared state on stable domain components or registries; per-client Resource wrappers forward operations to them. Cloud-backed state flows from cloud sync into the Go provider and ObjectStore caches, then through watches into React. Hash changes and session WebSocket notifications invalidate those caches.

A proto3 bool can arrive as `undefined` in TypeScript. Normalize it with `field ?? false` or `!!field`, and test the containing message for `null` to detect loading.

All Spacewave Cloud HTTP traffic goes through `core/provider/spacewave/client.go`. Typed API bodies use the generated proto-binary codecs with `Content-Type: application/octet-stream`, including empty acknowledgements. Call `doPostBinary`, `doGetBinary`, `doDelete`, `doPostStream` or `doMultiSig` to match the route. Bulk routes carry raw streams. Cloud WebSocket frames are binary envelope protos with oneof bodies.

## Data and generated sources

SharedObject IDs are lowercase ULIDs. A SharedObject's block-store ID is that ULID unchanged; `SobjectBlockStoreID(soID)` expresses the relationship, and cloud `bstoreId` parameters take `soID`.

Pass the mounted volume's `vol.GetID()` to `volume.ExBuildObjectStoreAPI`. Plugin-host proxy volumes can change IDs, and a reconstructed underlying ID can hang alias matching.

World object keys have the form `<stable-type-root>/<self-contained-id>`, using the object's own opaque or natural identity. Relationships between objects belong in graph edges; add a key-valued field only when a consumer needs that direct reference. A parent-scoped key may describe a bounded set of owned children. Each key builder has a parser for its own grammar. A durable key change needs a migration; an authorized rename uses `RenameObject(descendants=true)` and updates the graph quads that contain the key.

No World object type is a singleton. A user may create as many objects of any type as they want, so never derive a well-known key per Space, per user or per parent, and never assume a type has one instance. A consumer finds the objects that apply to it by following graph edges, such as an edge from a session to the policy object that governs it. An object with no edges affects nothing.

Block-backed state forms a block DAG under its World object. Create a separate World object when the state needs its own identity, permissions, lifecycle or graph relationships. Block DAG comments state the key encoding and value type.

- Proto imports use the Go module paths from `go.mod`.
- `sdk/` proto packages use the full `s4wave.` prefix; `core/` packages use shorter names. Qualify a type from another package fully, with a leading dot.
- Use `sdk/world/world.proto` and `sdk/world/` as the reference for resource services, request and response names, resource IDs and SDK wrappers.
- With `aptre`, stage changed `.proto` files before `bun run gen`. Regenerate every affected source; use `bun run gen:force` only when a forced rebuild is required.
- When the generator rewrites files outside your change, as after a generator version update, commit every rewritten file with your change. Keep generated output as the generator wrote it.
- Reuse the generated codecs, enums and domain types. Parse stable external payloads into typed fields; a raw payload may accompany them as debugging evidence.

## Storage and controller lifetimes

A Space can hold unbounded data. Opening a store, volume, World or index, a commit, and any work under a writer or engine lock must scale with the change or the answer, never with the stored data. Read through stored indexes, and look up one list in another through a map or set: a `slices.Contains` inside a loop or a `DeleteFunc` callback costs the product of both lengths. A test at small sizes cannot show this cost, so when you change such a path, measure it at production size, with hundreds of thousands of runs, keys or blocks.

Hold no lock longer than a second: not a SQLite write transaction or checkpoint, a writer turn, a mutex, nor a gate such as an eviction or move gate. Long work, such as an upload, an index of a pack, an eviction pass, a walk or a migration, runs as bounded steps, each taking the lock, doing a fixed amount of work, committing and releasing it before the next. Do network reads, bucket calls, fsyncs of large files and waits on other writers outside every lock. A step that must not repeat after a crash writes a marker first, in the same transaction as its first change, naming what it is doing, so a restart, even a killed process, finds the marker and finishes or undoes the step instead of redoing finished work. A step that meets a busy lock retries itself in place, never the whole job. Keep a counter of holds over a second, and treat any as a bug.

`BeginReadOperation`, `NewTransaction(false)`, bucket cursors, GC wrappers, projection hydration and resource read scopes can hold transaction locks. Reuse the existing scope when you layer stores. A second read transaction on the same bbolt or kvtx store while the first is open can deadlock when the mmap grows.

`broadcast.Broadcast` combines shared state with change notification. Read the state and take its wait channel inside one `HoldLock`, and emit directive values outside the lock.

A controller's `Execute(ctx)` receives its lifecycle context. It may wire subsystems to that context and return nil. Cleanup on controller removal belongs in `Close()`, so an `Execute()` that returns must not defer cancelling those subsystems. An error from `Execute()` restarts it with backoff on the same controller instance.

Resolvers use `directive.NewValueResolver` for static values and `directive.NewFuncResolver` for simple asynchronous resolution. A watch clears its values, snapshots the state and its wait channel, emits outside the lock, marks itself idle, then waits for a notification or cancellation. `HandleDirective` matches a directive without reading transient state.

Use the existing `Ex*` and `loader.WaitExecControllerRunning*` helpers. Keep a returned live `directive.Reference` until the caller's resource or lifecycle releases it.

## World writes and reads

Express each World change as a typed, registered `world.Operation` that records intent, applied through `ApplyWorldOp` or an object's `ApplyObjectOp`. Do not use an operation that only carries a list of final values. The operation carries every input its apply needs: the actor, timestamps, generated identities and payload refs. That way validation, the locked apply and a replica's replay decide alike. An apply reads only the World and the operation. Write the blocks an operation publishes before applying it, and run network calls, clock reads and other external effects outside the apply.

One writer holds a World's writer slot at a time, so every cold block read under it delays every other write. Do the slow reads before taking the slot, then validate again under it, because the head may have moved. Keep the locked part bounded: never hold a write transaction across a scan, a file transfer, a network call or a wait. Maintenance prepares its changes outside the slot and commits them in small batches that yield to foreground writes.

Exercise a new operation through the real apply path at representative and high cardinalities before landing it. Record the records and blocks it touches, how long it holds the writer slot, and its commit cost. An in-memory benchmark measures CPU, not storage latency.

Startup and opening must not depend on how much the World holds. A pass over stored records, such as a backfill or a cleanup, runs in the background after the component serves, and logs how many records it read and how long it took. A request, watch or loop starts at the objects it concerns and follows graph edges or an index. It never scans every object of a type, and it never repeats a lookup inside a loop over records.

A graph lookup is indexed by its subject or its object. A lookup that fixes only the predicate reads every quad in the World, however few edges match. For a World-wide set, link every member to one index object and read from that object; a self link is not an index. A watch, lease or cache that recomputes on every World revision multiplies a read's cost by its subscribers and the revision rate, so state that product before adding a read there.

A dependency update that changes a store's on-disk format converts it on first open. Measure that first open on a copy of production size before landing the update.

## CDN Spaces, releases and relays

A CDN Space's packs are content addressed and immutable; its root pointer changes on every commit. Serve packs with a long immutable cache rule, and serve the pointer from its own base URL (`RootPointerBaseURL`) with revalidation on every read, never behind the pack rule. Packs are append-only: a commit never replaces or deletes a pack that a published pointer names, and only the repack and reclaim path frees space, after the new pointer is published. A commit adds a pack only after its size and hash are verified, and the new pointer becomes visible only once it is durable.

Bucket credentials stay on the server. A writer uploads through a short-lived presigned URL, and a reader of a private Space gets a download authorization scoped to that Space's prefix. Neither ever holds the bucket key. A volume serves any block it holds to anyone who names its hash, so keep public and private content on separate volumes.

A launcher adopts a signed release selector and fetches its root at once. Check the selection out of the release World before signing it, and sign and serve the selector only after its root is durable on the CDN Space. A published revision only grows: roll back by reissuing a retained manifest at a revision above the current one, never by republishing an old revision. Keep a fixed number of revisions per platform so the release World stays bounded.

Supply relay credentials through `LookupICEProvider`, answered only for the local session's own peer. The provider token stays on the issuing server. Keep issued credentials in memory for one connection attempt, refuse those about to expire, and never persist, log or include them in directive debug values.

## Documentation audiences

Each page in `app/docs/content/` serves one audience:

- `users/`: say what to click and what happens, in the words the app shows. Drive, Space and `getting-started.md` belong here. Internal names such as SharedObject, World, UnixFS and provider IDs belong in developer pages. Keep CLI commands, flags, paths and environment variables exact, because users type them.
- `self-hosters/`: explain disks, backups, servers and recovery in the product's operational names.
- `developers/`: explain APIs, packages, protocols and internal mechanisms by their exact technical names.

## Formal models

Use TLA+ for critical behavior, concurrency, protocols, storage ordering and
lifecycles. Put each `<Name>.tla` and its bounded `<Name>.cfg` configurations
beside the code it models. Model the current implementation's callers,
interleaving steps, cancellation and failure paths. State the abstraction and
bounds, check safety and relevant liveness with `bun run check:tla`, and test
important guards with failing mutants outside the passing configurations.

Lean supplements TLA+ when a mathematical proof adds value: cryptographic
security assumptions and authorization, configuration-chain induction,
deterministic ordering, quorum intersection and consensus agreement. Keep
those proofs and their Go conformance checks focused on the mathematical
contract. Use TLA+ for host, worker, transport and storage lifecycle models.
A component may have both Lean and TLA+ models, or TLA+ alone; existing Lean
coverage is a reason to retain useful proofs and add temporal coverage too.
