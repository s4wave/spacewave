![Hydra](./doc/img/hydra-logo.png)

## Introduction

**Hydra** is a modular peer-to-peer data store with block-dag data structures:

 - **Advanced Structures**: block-graph SQL, Git, Graph DB, Files, k/v...
 - **Cross-Platform**: supports web-browser, native process, mobile, embedded...
 - **Data-exchange**: leverage optimized data transfer paths between volumes.
 - **Encryption**: transformations can encrypt or compress data at rest.
 - **Identity**: each volume has an identity independent from the host.
 - **Replication**: bucket policies implement data replication behaviors.

Stores peer-to-peer data structures on pluggable storage backends like [bbolt]
and [IPFS], as well as over 40 cloud storage providers (via [rclone]). Supports
nested and peer-to-peer block-graph backed volumes.

[bbolt]: https://github.com/etcd-io/bbolt
[IPFS]: https://ipfs.io/
[rclone]: https://github.com/rclone/rclone

## Overview

Hydra is built on the [ControllerBus] framework, which defines the Config,
Controller, Directive structures and behaviors.

[ControllerBus]: https://github.com/aperturerobotics/controllerbus

It uses the [Bifrost] network engine for communication between peers.

[Bifrost]: https://github.com/aperturerobotics/bifrost

The core storage system is implemented as:

 - **Volume**: common storage volume interface for persistence of data.
 - **Block**: a chunk of data hashed and identified by content-ID (see: IPFS).
 - **Bucket**: collection of blocks with attached data management policies.
 - **Reconciler**: bucket changes mqueue with a lazy-loaded controller.
 - **Object**: a pointer to a Block DAG located in a Bucket with transforms.
 
The block provides a `Cursor` for reading and modifying block DAGs. Most data
structures have block-DAG / block-cursor implementations:

- **Bitset**: bitset backed with a uint64 array.
- **Blob**: split a large piece of data into deterministic chunks.
- **Bloom**: bloom filter for efficient presence checking.
- **Fibheap**: efficient min() queries on a k/v heap.
- **Kvtx**: transactional key/value store (i.e. AVL tree).
- **MQueue**: FIFO message queue.
- **Msgpack**: blob encoded with the Msgpack protocol.
 
The following high-level data structures are implemented:

- **File**: collection of written Ranges composed of Blobs of data.
- **Git**: code revision tracking engine with go-git.
- **Graph**: graph database w/ quads: `<subject, predicate, object, value>`
- **Sql**: SQL data store backed by GenjiDB or go-mysql-server.
- **UnixFS**: directories, files, permissions, FUSE mounts.
- **World**: key/value store coupled with a graph database + changelog. 

For more details, see the [design overview](./doc/design.md).

An EntityGraph controller is provided, exposing the internal state of Hydra and
other systems to visualizers via a graph-based entity model.

## Volumes

Hydra assigns a persistent public/private keypair and peer ID to volumes.

The following volume types are currently implemented in this repository:

 - [BadgerDB]: high performance on-disk key/value data store.
 - [Block]: nested volume backed by a peer-to-peer block graph.
 - [BoltDB]: embedded key/value data store, using bbolt.
 - [In-memory]: in-memory key/value store for temporary data.
 - [IndexedDB]: with GopherJS/WASM in the web browser.
 - [RPC]: access a Volume on a remote Bus via a RPC service.
 - [Redis]: key/value storage with a remote Redis database.
 - [World]: nested volume backed by Object in a shared World.

The [volume controller] accepts any implementation of the [Store] interface.

Any key/value store can be used as a Volume by implementing the [kvtx] interface
and constructing the volume controller with the [kvtx-backed Store]. [In-memory]
key-value volume is an example of this approach.

Volumes can be nested: the [Block] volume uses the [kvtx/block] key/value store
to create a shared / networked Volume, which can be encrypted or compressed by
adding a [transform config]. The [World] volume stores data in a shared Object.

[BadgerDB]: ./volume/badger/badger.proto#L10
[Block]: ./volume/block/volume.proto#L11
[BoltDB]: ./volume/bolt/bolt.proto#L9
[In-memory]: ./volume/kvtxinmem/kvtxinmem.proto#L9
[IndexedDB]: ./volume/js/indexeddb/indexeddb.proto#L10
[Redis]: ./volume/redis/redis.proto#L10
[RPC]: ./volume/rpc/volume.proto
[Store]: ./store/store.go
[World]: ./volume/world/volume.proto#L10
[kvtx-backed Store]: ./store/kvtx
[kvtx]: ./kvtx/kvtx.go
[kvtx/block]: ./kvtx/block/kvtx.go
[transform config]: ./block/transform
[volume controller]: ./volume/controller

## Examples

Hydra can be used as either a [Go library] or a command-line / daemon.

[Go library]: ./examples/cross-platform/main.go

```bash
GO111MODULE=on go install -v github.com/aperturerobotics/hydra/cmd/hydra
```

Access help by adding the "-h" tag or running "hydra help."

Launch the daemon (uses hydra_daemon.yaml on default):

```
hydra daemon
```

The full list of available daemon CLI flags is currently:

```
OPTIONS:
   --badger-db value [ --badger-db value ]              set a path to a badger db dir to load on startup [$HYDRA_BADGER_DB]
   --bolt-db value [ --bolt-db value ]                  set a path to a bolt db file to load on startup [$HYDRA_BOLT_DB]
   --bolt-db-verbose                                    if set, mark bolt database as verbose (default: false) [$HYDRA_BOLT_DB_VERBOSE]
   --redis-url value                                    set a url to a redis instance to connect to on startup [$HYDRA_REDIS_URL]
   --inmem-db                                           if set, start a in-memory volume on startup (default: false) [$HYDRA_INMEM_DB]
   --inmem-db-verbose                                   if set, mark inmem database as verbose. implies --inmem-db (default: false) [$HYDRA_INMEM_DB_VERBOSE]
   --node-priv value                                    path to node private key, will be generated if doesn't exist (default: "daemon_node_priv.pem")
   --api-listen value                                   if set, will listen on address for API connections, ex :5110 (default: ":5110")
   --prof-listen value                                  if set, debug profiler will be hosted on the port, ex :8080
   --config value, -c value                             path to configuration yaml file (default: "hydra_daemon.yaml") [$HYDRA_CONFIG]
   --write-config                                       write the daemon config file on startup (default: false) [$HYDRA_WRITE_CONFIG]
   --hold-open-links                                    if set, hold open links without an inactivity timeout (default: false) [$BIFROST_HOLD_OPEN_LINKS]
   --websocket-listen value                             if set, will listen on address for websocket connections, ex :5111 [$BIFROST_WS_LISTEN]
   --udp-listen value                                   if set, will listen on address for udp connections, ex :5112 [$BIFROST_UDP_LISTEN]
   --establish-peers value [ --establish-peers value ]  if set, request establish links to list of peer ids [$BIFROST_ESTABLISH_PEERS]
   --udp-peers value [ --udp-peers value ]              list of peer-id@address known UDP peers [$BIFROST_UDP_PEERS]
   --websocket-peers value [ --websocket-peers value ]  list of peer-id@address known WebSocket peers [$BIFROST_WS_PEERS]
   --pubsub value                                       if set, will configure pubsub from options: [floodsub, nats] [$BIFROST_PUBSUB]
```

If `--write-config` is set, the options configured on the CLI will be written to
the config YAML file: `hydra_daemon.yaml` on default.

The CLI arguments are provided for convenience: the YAML configuration format
allows an infinite number of concurrent controllers to be configured: for
example, several backing volumes, network transports, and app controllers.

### APIs and Client CLI

The client CLI has the following help output:

```
USAGE:
   hydra client command [command options] [arguments...]

COMMANDS:
   block                 volume bucket handle block sub-commands
   bucket                bucket store sub-commands
   object                object store sub-commands
   volume                volume sub-commands
   controller-bus, cbus  ControllerBus system sub-commands.
   bifrost               Bifrost network-router sub-commands.
```

This is an example of configuring a bucket and storing a block:

```sh
  ./hydra client bucket config -f ../../examples/bucket-configs/basic-1.json  --volume-regex ".*"

  echo "hello world" | ./hydra client block \
    --bucket-id bucket-basic-1 \
    --volume-id default \
    put -f "-"

  # The data hash is printed:
  # 2W1M3RQW6kLcw6kLCNWw9mA1pWRqGGFv9NxmjXNjjWjj6iLVLJM4

  # Now we can lookup the block 
  ./hydra client block \
    --bucket-id bucket-basic-1 \
    get --ref 2W1M3RQW6kLcw6kLCNWw9mA1pWRqGGFv9NxmjXNjjWjj6iLVLJM4
```

To store data into the key/value store:

```sh
  ./hydra client object \
    --store-id store-basic-1 \
    --volume-id default \
    put --key "test" -f cmd_client.go
  ./hydra client object \
    --store-id store-basic-1 \
    --volume-id default \
    get --key test
```

Demonstration of data exchange between two peers:

```sh
  # configure the lookup via pubsub
  hydra client bucket config -f ../../examples/bucket-configs/psecho-1.json  --volume-regex ".*"
  
  # put a block on one peer
  echo "hello world 123" | hydra client block \
    --bucket-id bucket-psecho-1 \
    --volume-id default \
    put -f "-"

  # on the other peer
  hydra client block \
    --bucket-id bucket-psecho-1 \
    get --ref 2W1M3RQWBWZxSFDV91oXXsVay12Nho1K4dvnVNZjkoCzR8Gix5xr
```

See the [Bifrost] docs for how to configure the peers to connect to each other.

For a daemon status output, use `hydra client cbus bus-info`:

```
✓ controller-bus running
Controllers:
        controllerbus/loader 0.0.1
        controllerbus/resolver/static 0.0.1
        hydra/entitygraph/reporter 0.0.1
        controllerbus/configset 0.0.1
        entitygraph/collector 0.0.1
        hydra/daemon/api 0.0.1
        bifrost/transport/udp 0.0.1
        hydra/volume/bolt 0.0.1
        hydra/world/block/engine 0.0.1
        bifrost/floodsub 0.0.1
        hydra/dex/psecho 0.0.1
        hydra/lookup/concurrent 0.0.1
[...]
```

### YAML Configuration File

The [ConfigSet] YAML format is defined by ControllerBus for specifying
controllers to load and run concurrently with associated configurations.

[ConfigSet]: https://github.com/aperturerobotics/controllerbus#configset

For example:

```yaml
# In the below example, "my-bolt-db-volume" is the unique ConfigSet controller ID.
# If multiple ConfigSet are applied with the same ID, the config with the highest revision will be used.

# Starts a bbolt database at a path.
my-bolt-db-volume:
  id: hydra/volume/bolt
  config:
    path: data.bbolt
    volumeConfig:
      volumeIdAlias: ["default"]
    verbose: true

# Starts the floodsub implementation of pub-sub.
# Also available: nats
pubsub:
  id: bifrost/floodsub
  config: {}

# Listen for incoming UDP connections (w/ Quic) on port 5112
udp:
  id: bifrost/udp
  config:
    dialers: {}
    listenAddr: :5112

# Create a simple storage bucket on startup.
# Add it to all loaded volumes.
create-mybucket:
  id: hydra/bucket/setup
  config:
    applyBucketConfigs:
    - volumeIdList:
      - "default"
      config:
        id: example-bucket-1
        version: 1

# Configure the "psecho" data-exchange controller.
# Serves & fetches data lookup searches over a pub-sub channel.
# Nodes will connect directly to each other to transfer data.
dex:
  id: hydra/dex/psecho
  config:
    bucketId: example-bucket-1
    pubsubChannel: example-psecho-1-ch

# Create an example data structure: load a Hydra "World Engine"
# The world stores a k/v tree of Objects with a Graph DB.
world-example:
  id: hydra/world/block/engine
  config:
    engineId: example-1
    bucketId: example-bucket-1
```

## Related Projects

The following Aperture Robotics components are dependencies, and their clients
are included in the client bundle:

 - [ControllerBus]: similar to microservices - communicating controllers.
 - [Bifrost]: networking components and engine built with ControllerBus.

[ControllerBus]: https://github.com/aperturerobotics/controllerbus
[Bifrost]: https://github.com/aperturerobotics/bifrost

## Testing

The "testbed" package provides a standard cross-platform ephemeral test setup
with in-memory storage and a reasonable set of default controllers loaded.

It is used across the Hydra project to write end-to-end unit tests:

```go
// TestWorld performs a simple test of operations against world.
func TestWorld(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Construct a new testbed
	tb, err := testbed.NewTestbed(ctx, le)

	// Construct a new storage cursor
	ocs, err := tb.BuildEmptyCursor(ctx)
	defer ocs.Release()

	// Construct a block transaction
	btx, bcs := ocs.BuildTransaction(nil)
	bcs.SetBlock(NewExampleBlock())
	// can use bcs.SetRef ....

	// Write the block structure to storage.
	rootRef, bcs, err = obtx.Write(true)
}
```

You can run the tests with `go test ./...`

## Developing

To re-generate the protobufs:

```
git add .
make gengo
```

To lint the code:

```
make lint
```

Re-generating protobufs is only necessary if they were changed.

## Developing on MacOS

On MacOS, some homebrew packages are required for `yarn gen`:

```
brew install bash make coreutils gnu-sed findutils protobuf
brew link --overwrite protobuf
```

Add to your .bashrc or .zshrc:

```
export PATH="/opt/homebrew/opt/coreutils/libexec/gnubin:$PATH"
export PATH="/opt/homebrew/opt/gnu-sed/libexec/gnubin:$PATH"
export PATH="/opt/homebrew/opt/findutils/libexec/gnubin:$PATH"
export PATH="/opt/homebrew/opt/make/libexec/gnubin:$PATH"
```

## Support

Please open a [GitHub issue] with any questions / issues.

[GitHub issue]: https://github.com/aperturerobotics/hydra/issues/new

... or feel free to reach out on [Matrix Chat] or [Discord].

[Discord]: https://discord.gg/KJutMESRsT
[Matrix Chat]: https://matrix.to/#/#aperturerobotics:matrix.org
