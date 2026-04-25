---
title: World and Object Model
section: internals
order: 7
summary: World, Objects, blobs, block stores, and volumes.
---

## Overview

Spacewave's data model is built on hydra, a content-addressed block-DAG storage system. User data, object state, and settings are stored as objects within a world. A world is a mutable state container backed by an immutable block store. This architecture provides local-first operation, cross-device sync, and verifiable data integrity.

## Worlds

A world is the top-level state container for a space. It holds a directed acyclic graph of objects and provides transactional operations for reading and writing state. Each world has a sequence number that increments on every mutation, enabling watchers to detect changes efficiently.

The `WorldState` interface provides access to world data:

- `GetSeqno()` - Returns the current sequence number.
- `WaitSeqno()` - Blocks until the sequence number reaches a threshold.
- `BuildStorageCursor()` - Opens a cursor to the world's bucket storage.
- `ApplyWorldOp()` - Applies a typed operation (create object, update settings, etc.).

World operations are identified by operation type IDs (e.g., `SET_SPACE_SETTINGS_OP_ID`, `INIT_UNIXFS_OP_ID`). Each operation type has a corresponding protobuf message and handler. This design allows plugins to register custom operation types that extend the world's mutation vocabulary.

## Objects

An object is a named entry in a world. Objects are identified by a string key (e.g., `docs/documentation`, `vm/v86`) and have a type that determines how they are rendered and managed. Objects store their state as protobuf messages within the world's bucket storage.

Object types are registered by plugins. When a space contains an object of a given type, the runtime loads the plugin that handles that type. The plugin provides a viewer component for rendering and optionally a backend service for custom operations.

## Blocks and Content Addressing

All data is stored as content-addressed blocks. A block is an immutable byte array identified by its cryptographic hash (multihash). Blocks are organized in a Merkle DAG where each block can reference other blocks by their hash. This structure provides:

- **Deduplication** - Identical data produces identical hashes, stored once.
- **Integrity** - Any modification changes the hash, making tampering detectable.
- **Sync** - Devices exchange blocks they are missing, not entire datasets.

The block store interface (`block.StoreOps`) exposes `PutBlock`, `GetBlock`, `GetBlockExists`, `StatBlock`, and `RmBlock` operations. Block references include the hash type, digest, and block size.

## Buckets

A bucket is a namespace within a volume's block store. Buckets provide key-value access on top of the block layer. Objects reference data within buckets using `ObjectRef` values that contain the bucket ID and a block reference.

The bucket lookup cursor (`bucket_lookup.Cursor`) provides scoped access to a world's storage. It resolves object references relative to the world's root bucket and supports iteration over the world's object graph.

## Volumes

A volume is a named storage backend identified by a volume ID (e.g., `hydra/volume/bolt/12D3KooW...`). Volumes encapsulate the block store, peer identity, and hash configuration. Multiple volume implementations exist (BoltDB, SQLite, in-memory, browser-based), all conforming to the same `Volume` interface.

Each volume has an associated peer identity (a libp2p key pair). The peer ID is used for signing world transactions and authenticating data during sync. Volume info includes the volume ID, peer ID, public key, and hash type.

## Graph Predicates

Objects in a world are connected through graph predicates using the Cayley graph database. Predicates express relationships like "object A links to object B with tag `<manifest>`". The `plugin/host/scheduler` controller uses these predicates to discover plugin manifests linked from a root object key.

## Next Steps

- [Storage and Volume RPC](/docs/developers/internals/storage-and-volume-rpc) for the RPC surface that exposes volume operations to plugins.
- [Resource System](/docs/developers/sdk/resource-system) for how the SDK accesses world state from TypeScript.
