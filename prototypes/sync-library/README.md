# Sync Library Prototype

A Spacewave world compiled to JavaScript as an embeddable sync store:
create-or-open a key/value collection, mutate transactionally, and
subscribe to live snapshots - the ElectricSQL/Replicache shape on the
Hydra block DAG.

## Compile

From this repository root (binding roots point at dependency checkouts
carrying es-lite `.pb.ts` siblings; see review 20260821):

    bin/goscript compile \
      -p ./prototypes/sync-library/lean \
      --all-dependencies \
      -b '-tags=goscript,skip_e2e,purego' \
      --protobuf-ts-binding \
      --binding-root <cayley-checkout> \
      --binding-root <protobuf-go-lite-checkout> \
      --compiler-cache-root /tmp/fresh-cache \
      --output .tmp/kvlib

Link the emitted tree for module resolution:

    cd .tmp/kvlib
    mkdir -p node_modules/@goscript
    for d in @goscript/*/; do ln -sfn "$(pwd)/${d%/}" "node_modules/@goscript/$(basename "${d%/}")"; done

## Run (bun, direct ESM)

    bun kv-demo.mjs

Expected output: live snapshot counts after each put/delete, get and list
round-trips, `kv-demo OK`.

## API

| Function | Behavior |
|---|---|
| `KvOpen(ctx)` | open in-memory world + default KV store |
| `KvOpenDurable(ctx, dir)` | durable volume: OPFS on js targets, bbolt elsewhere |
| `KvPut(key, value)` | set value (string or bytes) |
| `KvGet(key)` | value string, empty when absent |
| `KvExists(key)` | presence check |
| `KvDelete(key)` | delete if present |
| `KvList(prefix)` | JSON array of `{key, value}` under prefix |
| `KvWatch(prefix, cb)` | live snapshots after every commit |
| `KvStopWatches()` / `KvClose()` | tear down subscriptions / everything |

Known teardown gap: after `KvClose()` a live bus handle keeps the JS event
loop alive; end scripts with an explicit `process.exit(0)` until resolved.

## Bundler recipes

1. **bun direct ESM** (recommended): run the compiled tree directly.
   Works bound (`--protobuf-ts-binding`) and unbound.
2. **node via rolldown bundle**: works in unbound mode. Use a rolldown
   `resolveId` plugin mapping `@go/<import-path>` onto
   `<output>/@goscript/<import-path>` (see `.tmp` build scripts), then
   `node bundle.mjs`. Proven with the gonum-free tree.
3. **bound-mode node bundling**: currently broken - the protobuf-es-lite
   enum descriptor path fails at runtime because generated `_srpc`
   modules mix with es-lite modules. Needs either `_srpc` binding support
   in goscript or bundler dedup rules.

## Hosted mode

Run the Go server (native binary, hosts the authoritative world):

    go run ./prototypes/sync-library/hosted/server -addr :8900

Connect a compiled client to it instead of opening an embedded world:

    await KvOpenHosted(ctx, 'ws://127.0.0.1:8900/ws')

The same `Kv*` functions then operate against the hosted authoritative
state. Cross-process proof lives in
`hosted/client/kv-hosted-demo.mjs`: the client subscribes before the
server's delayed write and observes it through WatchPrefix.

## Durability

- js targets: OPFS volume (`volume/js/opfs`), mirroring
  `bldr/storage/browser` wiring. Runtime verification requires a browser
  harness (OPFS is not available in Node).
- other platforms: bbolt via `KvOpenDurable(ctx, dir)`; close/reopen test
  in `lean/kvapi_test.go`.
