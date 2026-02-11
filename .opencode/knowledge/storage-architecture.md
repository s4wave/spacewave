# Hydra Storage Architecture

## Overview

Hydra is a modular P2P data store with block-DAG structures. Storage is layered:

- **KVtx Store** - Universal transactional K/V abstraction (the core interface everything builds on)
- **Block Store** - Content-addressed block DAG on top of KVtx
- **SQL Store** - Relational queries on top of blocks (not a standalone store)
- **World/Graph** - Semantic entity relationships on blocks
- **Bucket/Object/Volume** - Higher-level storage management

## KVtx Interface (`store/kvtx/kvtx.go`)

The central abstraction all backends implement:

```
Store:
  NewTransaction(ctx, write bool) -> Tx

TxOps:
  Get(ctx, key) -> (data, found, error)
  Exists(ctx, key) -> (bool, error)
  Set(ctx, key, value) -> error
  Delete(ctx, key) -> error
  ScanPrefix(ctx, prefix, callback) -> error
  ScanPrefixKeys(ctx, prefix, callback) -> error
  Iterate(ctx, prefix, sort, reverse) -> Iterator
  Size(ctx) -> (count, error)

Iterator:
  Next() -> bool, Seek(k), Valid(), Key(), Value(), ValueCopy(), Close()
```

## KVtx Backend Implementations

| Backend | Location | Platform | Notes |
|---------|----------|----------|-------|
| **BadgerDB** | `volume/badger/` | Native | LSM tree, value log separation |
| **BoltDB** | `volume/bolt/` | Native | Single-file B+ tree, ACID |
| **SQLite** | `volume/sqlite/` | Native | Dual driver (CGO + pure-Go) |
| **Redis** | `volume/redis/` | Network | Remote, in-memory + persistence |
| **In-Memory** | `store/kvtx/inmem/` | Any | HashMap, testing |
| **Ristretto** | `store/kvtx/ristretto/` | Native | Concurrent cache w/ bloom filter |
| **KVFile** | `store/kvtx/kvfile/` | Any | Read-only index over binary file |
| **IndexedDB** | `volume/js/indexeddb/` | Browser/WASM | Browser storage |

## Platform Selection (build tags)

```
Native (default):  BoltDB  (!js && !redis)
Native + Redis:    Redis   (!js && redis)
Browser/WASM:      IndexedDB (js)
```

SQLite is explicitly excluded from JS/WASM (`!js && !wasip1`).

## Block Store on KVtx (`block/store/kvtx/`)

Blocks are stored as pure K/V: `hash(block) -> serialized_block`. Content-addressed, write-once, immutable.

Block transactions use merkle-DAG with topological sorting:
1. Build in-memory graph of block references
2. Topologically sort (gonum) to determine write order
3. Concurrent encode pipeline (GOMAXPROCS workers)
4. Concurrent write pipeline to backing KVtx store

## Access Patterns

1. **Point reads** - Block retrieval by hash (Get)
2. **Sequential batch writes** - Block transaction commits (topologically ordered)
3. **Prefix range scans** - State traversal, reconciliation (ScanPrefix/Iterate)
4. **Transactional multi-key** - ACID within single Tx boundary

## SQLite Backend Details (`store/kvtx/sqlite/`)

**Schema:** `CREATE TABLE t (key BLOB PRIMARY KEY, value BLOB)`

**Drivers:**
- CGO: `mattn/go-sqlite3` (build tag: `cgo && !js && !wasip1`)
- Pure Go: `modernc.org/sqlite` (build tag: `!cgo && !js && !wasip1`)
- Automatic selection via build tags

**Features:**
- WAL mode enabled by default
- Write transactions acquire RESERVED lock early via dummy DELETE
- Retry logic: up to 5 retries on SQLITE_BUSY with exponential backoff
- Iterator: precomputed SQL queries, O(log n) seeks, prefix range via `key >= ? AND key < ?`
- Concurrent readers allowed, writers serialized

**Config proto:** `volume/sqlite/sqlite.proto` - path, table name, key opts, verbose logging

## SQL Store (`sql/mysql/`)

SQL is layered ON TOP of block storage, not standalone:
- Root cursor -> Database -> Tables -> Rows -> Columns
- Each node is a serializable block in the DAG
- Not used as a direct persistent store

## Volume Layer

All backends wrap in `kvtx.Volume` adding:
- KVKey encryption/derivation
- Optional verbose logging (vlogger wrapper)
- Transaction lifecycle management
- Private key storage
- Controller lifecycle via ControllerBus

## Transaction Semantics

- Explicit lifecycle: must call Commit() or Discard()
- Discard() is idempotent, safe to defer
- Read transactions: snapshot isolation, concurrent
- Write transactions: serialized (backend-dependent mechanism)
