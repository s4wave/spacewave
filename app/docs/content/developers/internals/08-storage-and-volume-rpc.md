---
title: Storage and Volume RPC
section: internals
order: 8
summary: Volume RPC surface and block store APIs.
---

## Overview

The volume RPC layer exposes hydra's storage primitives to plugins and remote clients over starpc streaming RPC. Plugins running in Web Workers or WASM sandboxes cannot access the volume directly. Instead, they interact with a proxy volume provided by the plugin host scheduler. The RPC surface covers volume discovery, block storage, bucket operations, and object stores.

## AccessVolumes Service

The `AccessVolumes` service defined in `hydra/volume/rpc/volume.proto` provides two RPCs:

**WatchVolumeInfo** - Streams volume metadata updates. Returns the volume ID, peer ID, public key PEM, and hash type. If the volume is not found, the response sets `not_found: true`.

```protobuf
rpc WatchVolumeInfo(WatchVolumeInfoRequest)
  returns (stream WatchVolumeInfoResponse);
```

**VolumeRpc** - Opens a bidirectional RPC stream to a specific volume. The stream acts as a multiplexed channel exposing the `ProxyVolume` service and additional sub-services on the volume.

## ProxyVolume Service

Once connected via `VolumeRpc`, the `ProxyVolume` service exposes:

| RPC | Description |
|-----|-------------|
| `GetVolumeInfo` | Returns the volume's peer ID, public key, and hash type |
| `GetPeerPriv` | Returns the volume's private key (if available) |
| `GetStorageStats` | Returns storage usage statistics |

The proxy channel also exposes several block-level services:

- **BlockStore** - `PutBlock`, `GetBlock`, `GetBlockExists`, `StatBlock`, `RmBlock` for content-addressed block operations.
- **BucketStore** - Key-value operations scoped to a named bucket within the volume.
- **MqueueStore** - Message queue operations for ordered event streams.
- **ObjectStore** - High-level object CRUD backed by the block store.

## Volume Identity

Each volume has a peer identity (libp2p key pair). The `VolumeInfo` message contains:

- `volume_id` - Unique identifier for the volume on the bus.
- `peer_id` - Base58-encoded peer ID derived from the public key.
- `peer_pub` - PEM-encoded public key for verifying signatures.
- `hash_type` - The preferred hash algorithm for new blocks (e.g., SHA2-256).

## Plugin Volume Access

When the plugin host scheduler runs a plugin, it creates a proxy volume on the plugin's bus. The proxy volume ID may differ from the original storage volume ID. Plugins must use `vol.GetID()` from the mounted volume reference rather than constructing volume IDs from raw storage identifiers.

```go
// Correct: use the mounted volume's actual ID
volume.ExBuildObjectStoreAPI(ctx, bus, false, objStoreID, vol.GetID(), cancel)

// Wrong: reconstructing the ID from parts
volume.ExBuildObjectStoreAPI(ctx, bus, false, objStoreID, StorageVolumeID(provID, accountID), cancel)
```

The proxy volume on the plugin bus has a bolt volume ID (e.g., `hydra/volume/bolt/12D3KooW...`), not the original storage volume ID (e.g., `p/local/{accountID}`).

## Block Store Operations

The core block store interface provides five operations:

- **PutBlock(data, opts)** - Stores a block and returns its content-addressed reference.
- **GetBlock(ref)** - Retrieves block data by reference. Returns `(data, exists, error)`.
- **GetBlockExists(ref)** - Checks existence without reading data.
- **StatBlock(ref)** - Returns block metadata (size) without reading data.
- **RmBlock(ref)** - Deletes a block from the store.

Block references contain the hash type, digest bytes, and block size. The hash type is determined by the volume's configuration.

## Volume Implementations

Hydra ships several volume backends:

| Backend | Use Case |
|---------|----------|
| BoltDB | Desktop and server deployments |
| SQLite | Desktop, mobile, and embedded |
| Browser (IndexedDB/OPFS) | Web browser runtime |
| In-memory | Testing and ephemeral storage |
| Redis | Server-side caching layer |

All backends implement the same `Volume` interface, making storage swappable without changes to application code.

## Next Steps

- [World and Object Model](/docs/developers/internals/world-and-object-model) for how worlds and objects are organized on top of volumes.
- [Resource System](/docs/developers/sdk/resource-system) for how the TypeScript SDK accesses storage.
