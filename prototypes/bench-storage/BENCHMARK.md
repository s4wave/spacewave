# Storage Benchmarks

Micro-benchmarks for in-browser WASM storage operations, measuring hashing
throughput, IndexedDB key-value read/write latency, and UnixFS file creation.

## Running

Requires a browser-based WASM test runner (wasmbrowsertest):

```bash
cd prototypes/bench-storage
GOOS=js GOARCH=wasm go test -bench ./ | sed '/could not marshal/d' > BROWSER_RESULTS.md
```

## Benchmarks

### Hash baselines

- **BenchmarkBlake3** - BLAKE3 hash throughput at 4 KiB, 64 KiB, 1 MiB.
- **BenchmarkSHA256** - SHA-256 hash throughput at the same sizes.

### IndexedDB KV write

- **BenchmarkIndexedDBKVWrite** - One write transaction per operation. Each
  iteration opens a write transaction, sets one key, and commits. Measures the
  combined cost of transaction creation, IndexedDB put, and transaction commit.

- **BenchmarkIndexedDBKVWriteSingleTx** - All writes in a single raw IndexedDB
  transaction (bypasses the txcache layer). The final commit happens after the
  timer stops. Isolates raw IndexedDB put latency from per-operation commit
  overhead. Comparison with the per-op variant shows how much time is spent in
  transaction commit.

### IndexedDB KV read

- **BenchmarkIndexedDBKVReadTxPerOp** - One read-only transaction per operation.
  Each iteration opens a transaction, reads one key, and discards. Measures
  per-transaction read overhead.

- **BenchmarkIndexedDBKVReadSingleTx** - All reads in a single read-only
  transaction. Isolates IndexedDB get latency from transaction creation overhead.

### UnixFS

- **BenchmarkIndexedDBUnixFSWriteFile** - Creates a file via MknodWithContent
  through the full world transaction stack. Each iteration opens a world
  transaction, writes a file node, and commits. Measures end-to-end file
  creation cost including block encoding and world state management.

## Motivation

IndexedDB transaction commit is suspected to be a major bottleneck. The
single-transaction variants quantify the commit overhead. If the difference is
large, keeping a persistent write transaction open and committing on idle (or
relying on auto-commit) could significantly improve throughput.
