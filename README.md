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
and [IPFS], as well as over 40 cloud storage providers (via [rclone]).

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

## Examples

Hydra can be used as either a Go library or a command-line / daemon.

```bash
GO111MODULE=on go install -v github.com/aperturerobotics/hydra/cmd/hydra
```

Access help by adding the "-h" tag or running "hydra help."

As a basic example, launch the daemon (use hydra_daemon.yaml from cmd/hydra):

```
hydra daemon
```

[Cross-platform example] of most of the Hydra APIs in use in a Go program.

[Cross-platform example]: ./examples/cross-platform/main.go

### YAML Configuration File

The ConfigSet YAML format is defined by ControllerBus for specifying controllers
to load and run concurrently with associated configurations:

```yaml
# In the below example, "my-bolt-db-volume" is the unique ConfigSet controller ID.
# If multiple ConfigSet are applied with the same ID, the config with the highest revision will be used.

# Starts a bbolt database at a path.
my-bolt-db-volume:
  id: hydra/volume/bolt/1
  config:
    path: data.bbolt
    verbose: true
  revision: 1

# Starts the floodsub implementation of pub-sub.
# Also available: nats
pubsub:
  id: bifrost/floodsub/1
  config: {}
  revision: 1

# Listen for incoming UDP connections (w/ Quic) on port 5112
udp:
  id: bifrost/udp/1
  config:
    dialers: {}
    listenAddr: :5112
  revision: 1

# Create a simple storage bucket on startup.
# Add it to all loaded volumes.
create-mybucket:
  id: hydra/bucket/setup/1
  config:
    applyBucketConfigs:
    - volumeIdRe: '.*'
      config:
        id: example-bucket-1
        version: 1
  revision: 1

# Configure the "psecho" data-exchange controller.
# Serves & fetches data lookup searches over a pub-sub channel.
# Nodes will connect directly to each other to transfer data.
dex:
  id: hydra/dex/psecho/1
  config:
    bucketId: example-bucket-1
    pubsubChannel: example-psecho-1-ch
  revision: 1

# Create an example data structure: load a Hydra "World Engine"
# The world stores a k/v tree of Objects with a Graph DB.
world-example:
  config:
    engineId: example-1
    bucketId: example-bucket-1
  id: hydra/world/block/engine/1
  revision: 1
```

### GRPC APIs and Client CLI

Most functionality is optionally exposed on the client CLI and GRPC API:

 - Bucket: create/update/delete
 - Block: into bucket: get/put/delete
 - Kvtx: (also called "Object Store"): get/list/put/delete.
 - Volume: list mounted volumes. Configure more using controller-bus API.

The client CLI has the following help output:

```
USAGE:
   hydra client command [command options] [arguments...]

COMMANDS:
   block                 volume bucket handle block sub-commands
   object                object store sub-commands
   apply-bucket-conf     Apply a bucket conf to one or more volumes.
   list-buckets          Lists local bucket info across multiple volumes.
   list-volumes          Lists local attached volume info.
   controller-bus, cbus  ControllerBus system sub-commands.
   bifrost               Bifrost network-router sub-commands.
```

Follow the following simple example:

```
  ./hydra client apply-bucket-conf -f ../../examples/bucket-configs/basic-1.json  --volume-regex ".*"
  # copy volume id into below command
  echo "hello world" | ./hydra client block \
    --bucket-id bucket-basic-1 \
    --volume-id hydra/bolt/12D3KooWJZ1SVqgT72WSmtdBH9vwhJpCEsrg2G1BcxgddTKiBThz \
    put -f "-"
  ./hydra client block \
    --bucket-id bucket-basic-1 \
    get --ref 2W1M3RQW6kLcw6kLCNWw9mA1pWRqGGFv9NxmjXNjjWjj6iLVLJM4
```

To store data into the key/value store:

```
  ./hydra client object \
    --store-id store-basic-1 \
    --volume-id hydra/bolt/12D3KooWJZ1SVqgT72WSmtdBH9vwhJpCEsrg2G1BcxgddTKiBThz \
    put --key "test" -f cmd_client.go
  ./hydra client object \
    --store-id store-basic-1 \
    --volume-id hydra/bolt/12D3KooWJZ1SVqgT72WSmtdBH9vwhJpCEsrg2G1BcxgddTKiBThz \
    get --key test
  # 2W1M3RQW6kLcw6kLCNWw9mA1pWRqGGFv9NxmjXNjjWjj6iLVLJM4
```

Demonstration of exchanging data between two peers:

```
  hydra client apply-bucket-conf -f ../../examples/bucket-configs/psecho-1.json  --volume-regex ".*"
  # copy volume id into below command
  echo "hello world 123" | hydra client block \
    --bucket-id bucket-psecho-1 \
    --volume-id hydra/bolt/12D3KooWJZ1SVqgT72WSmtdBH9vwhJpCEsrg2G1BcxgddTKiBThz \
    put -f "-"
  hydra client block \
    --bucket-id bucket-psecho-1 \
    get --ref 2W1M3RQW6kLcw6kLCNWw9mA1pWRqGGFv9NxmjXNjjWjj6iLVLJM4
  hydra client block \
    --bucket-id bucket-psecho-1 \
    get --ref 2W1M3RQWBWZxSFDV91oXXsVay12Nho1K4dvnVNZjkoCzR8Gix5xr
```

See the [Bifrost] docs for how to configure two peers to connect to each other.

For a simple daemon status output, use `hydra client cbus bus-info`:

```
✓ controller-bus running
Controllers:
        controllerbus/loader/1 0.0.1
        controllerbus/resolver/static/0.0.1 0.0.1
        hydra/entitygraph/reporter/1 0.0.1
        controllerbus/configset/1 0.0.1
        entitygraph/collector/1 0.0.1
        hydra/daemon/api/1 0.0.1
        bifrost/transport/udp/0.0.1 0.0.1
        hydra/volume/bolt/1 0.0.1
        hydra/world/block/engine/1 0.0.1
        bifrost/floodsub/1 0.0.1
        hydra/dex/psecho/1 0.0.1
        hydra/lookup/concurrent/1 0.0.1
[...]
```

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

## Support

Hydra is built & supported by Aperture Robotics, LLC.

Please open a [GitHub issue] with any questions / issues.

[GitHub issue]: https://github.com/aperturerobotics/hydra/issues/new
