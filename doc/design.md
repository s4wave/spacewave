# Additional Design Notes

This document contains some additional design notes and details.

Nodes in the network do not trust each other. Data must be verified when
performing or completing transfers and stores. Typically this verification
occurs by hashing the data and comparing the hash to the expected value. As
objects are stored with their hash as their ID (the content ID approach) it's
then impossible to read inconsistent data if we validate the hash at storage
time or read time (depending on the expected consistency of the store). With
this property, Hydra is similar to immutable stores such as Bittorrent.

## Code Organization

These are the types of implemented data structures:

 - block: content-ID reference graph
   - blob: split a blob of data into multiple blocks
   - file: copy-on-write file implementation using blobs
   - iavl: avl tree, implements kvtx
 - bucket: grouping of blocks in storage volume(s)
   - object: reference a block in a different bucket or with different
    transformation parameters
 - dex: data exchange protocols
 - git: stores a git repository with go-git
   - block: block graph implementation of git repo
 - heap: common interface for all heaps
   - heaptest: test for all heap stores
 - kvtx: transaction-based key/value store
   - block: backwards-compatible block-graph kvtx trees
   - cayley: graph database implementation
   - fibheap: Fibonacci priority heap
   - hidalgo: translates hidalgo interfaces to kvtx
   - iterator: consistent sorted iteration polyfill
   - kvtest: test for all kvtx stores
   - mqueue: FIFO message queue implemented with kvtx
   - prefixer: prepend a prefix to keys
   - txcache: buffer changes in memory before committing transaction
   - vlogger: log all actions to a logger handle
 - mqueue: message queue
 - sql: contains all SQL implementations
   - genji: genjidb (based on kvtx)
   - mysql: mysql-compatible protocol (based on go-mysql-server)
 - unixfs: unix filesystem using the block-graph file + blob store
   - fstree: filesystem tree block graph implementation
 - volume: management of a storage backend
 - world: graph database of object references w/ multi-source compositing
   - block: block graph implementation of world graph
 
Each of the top-level directories contains a declaration of a data structure
interface, with higher-level data structures implemented on top of the declared
data structure in sub-directories.

## Volume

A "Volume" is storage space in the form of memory, disk, or networked stores.
Each Volume is assigned a generated public/private keypair. Each Volume's
storage capacity, transfer rate, node association, identity, active transfers,
and other properties are tracked by the "Volume Controller." Requests to
manipulate data are interpreted and executed by the volume controller.

## Data Structures Pattern

The general pattern used for the data structures (i.e. git, world, mySQL):

 - State: underlying storage read/write of data.
 - Tx: transactional interface on top of the State.
 - Engine: has NewTransaction call to create Tx objects.
 - EngineState: implements State on top of Engine (auto-manage Txs).
 
The interfaces for the above are defined in the root package, and the
block-graph bindings are implemented in a "block" sub-package. The "engine" is
usually then implemented in the "block/engine" sub-package as a controller.

## Data consistency

Hydra implements a Write-Only-Read-Many model (WORM): a Block, once created, is
immutable. Blocks are referenced by their hash, using content identifiers. This
property allows the data to be efficiently transported through p2p networks such
as BitTorrent. Once a Block is created, it cannot be guaranteed that the Block
will ever cease to exist in the network, although policies can be adjusted to
trigger garbage collection of data.

## Block

A "Block" is a binary blob of arbitrary size. The underlying storage engine
works in terms of the basic Block primitive. Blocks can be encoded using any
encoding and/or compression algorithms. Blocks are hashed at storage time and
that hash is used as a content ID for the Block.

### Naming

Hydra is a content-identified data store, using unique properties of data
(hashes) to uniquely identify and name objects. No attempt is made to map
between human readable filenames and objects. The "unixfs" component implements
a FUSE-mountable unix filesystem on top of the Block store.

### Object

An Object is a set of blocks connected together with references to form a
persistent structure. Objects can point to other objects, forming a Directed
Acyclic Graph of references.

Objects are immutable once stored or referenced by a pointer. Modifying an
object requires creating a new object with the modifications applied. This also
requires that all objects pointing to the object be re-created with updated
pointers, and so-on, forming a Directed Acyclic Graph (DAG) of pointers.

Hydra has facilities for tracking modifications to object structures and
recursively applying changes to references through a DAG. These facilities also
include the ability to declare a set of transformations to be applied to each
block in a structure before writing or reading to/from storage. The
transformation structure can be used to implement at-rest encryption, as well as
any other data management approaches.

## Bucket

A "Bucket" is a collection of Block with associated configuration. Block objects
are placed into Buckets, and the Volume subsystem manages storing the Blocks
inside the Buckets in storage space. Buckets provide a logical container for
blocks within volumes, as well as a location to configure reconciliation
policies for changes to the bucket.

### Configuration

Bucket configurations have a revision ID. Newer revision IDs always take
precedence over older revision IDs.

Attached volume controllers can be instructed to ingest new bucket
configurations via a directive. Additionally, running volume controllers can be
queried for the state of a particular bucket. Each volume controller will attach
a bucket state object to these directives after processing the directive intent.

The volume controllers only validate the data in the configuration and the
property that greater revision numbers take precedence. It is up to other
abstraction layers - directives and controllers - to manage acquiring and
validating incoming bucket configurations.

### Lookup

The bucket configuration can specify a "lookup controller" to load to service
these requests on-demand, which enables full per-bucket customization of
behavior and dynamic behavior loading at runtime. The Hydra node-local
controller is responsible for servicing bucket lookup requests. A "Lookup" is
any request that targets a bucket across multiple local or remote volumes.

### Reconcilers

As data moves through the system, Events are generated, i.e. "block written to
Volume." Reconcilers process the event queue in order, with at-least-once
acknowledgment assurance. A filtering policy can be specified to filter events
from being passed to a particular reconciler as a significant optimization. When
a matching event is received, the reconciler is started/woken by the volume
controller. It then reads the oldest event from the front of the queue, writes
to an internal state representation or otherwise actuates changes, and finally
acknowledges the event.

Reconcilers are not terminated when their event queue becomes empty. However, if
a reconciler process exits cleanly while the event queue is empty, the
reconciler will not be restarted until the event queue is filled again.

This mechanism allows Hydra to avoid launching unnecessary computation for
dormant or archived data. Bucket reconciliation controllers are launched
on-demand and released when no longer needed. Multiple volumes with the same
bucket will launch a single routine to manage the concern.

#### Example: volume startup sequence

This sequence is used for processing bucket reconcilers:

 1. The volume `V_1` is mounted. If any bucket reconciler queues are filled,
    then proceed to step 4. If any reconcilers are marked as "run when idle,"
    they are also started by proceeding to step 4.
 2. Data is requested to be written into bucket `B_1`. The bucket configuration
    is loaded from the volume, if not already cached in memory (LRU map cache).
 3. Data is written into bucket `B_1` in volume. Before or atomically while the
    write is performed, an event representing the change is pushed into the
    bucket's reconcilers' queues.
 4. After the data is written completely, the event is fed to the running
    instance of the reconciler. This process includes waking up / starting the
    reconciler if it is not already running.
 5. The reconciler peeks the event, and internally writes to its state the
    requirement that it should push the object to the remote store.
 6. The reconciler requests that it remain running when "idle" as it has data to
    transfer internally.
 7. The reconciler acks the event, removing it from the volume queue.
 8. The reconciler finishes all queued transfers, exiting cleanly.

This is provided in the common "Volume Controller" implementation.

## Replication and synchronization

Replication reconcilers control how and in what order/priority a node will
attempt to replicate (copy) data between Volumes. Replication directives specify
where data should be stored in precise or general terms to be interpreted by
replication reconcilers in the network. Controllers may communicate to form a
local consensus of data placement and short-term planned data transfers to drive
the general network equilibrium towards the desired goal state.

## Fault detection

Fault detection is implemented by bucket reconcilers. This behavior is not
mandated nor understood by the core volume controllers, which think in terms of
reconcilers and events, rather than failures and data restoration.

When a peer wishes to transfer data into a remote bucket, the two peers involved
in the transaction communicate to share the reasoning behind the data placement,
including any justification for buckets that have been found to be offline. This
facilitates a gossip-like mechanism for propagating host failures across the
network. If a peer becomes aware of a failed remote bucket, it will attempt to
transfer the data to other known remote buckets to satisfy the replication
constraints. In the process of doing this "push" or "pull" of data between
locations, the original reasoning in the form of the knowledge that the original
bucket has disappeared propagates in-band as justification for the transfer.

## Kvtx: Key-value Transactions

The "kvtx/block" package contains a backwards-compatible implementation of
multiple key/value block-graph structures implementing kvtx. The current default
implementation is the AVL tree. Backwards compatibility is implemented with a
header specifying which block-graph k/v structure is in use.

The "kvtx/txcache" package buffers changes in memory as a compatibility shim for
stores which do not implement the commit/discard transaction mechanics.

Many other structures are implemented on top of the kvtx interfaces.

## World Graph

The Hydra "World Graph" is a key/value store coupled with a graph database. It
has an optional 2-dimensional changelog linked-list, containing a bloom filter
to quickly determine which keys were affected and filter unnecessary entries.

Multiple implementations are available, including a block-graph p2p DAG using
Cayley Graph and a configurable key/value store (see: kvtx).

## Concurrent Message Queue

A Message Queue is a store-backed, FIFO, at-least-once delivery, concurrent
reader and writer safe structure. An implementation is provided in the
"kvtx/mqueue" package.

## SQL Implementation

The MySQL-compatible implementation under `sql/mysql` uses the `go-mysql-server`
project mapped to a block DAG data structure. Msgpack is used to encode field
type information, and Blobs are used to chunk & store larger values.

Bindings for the Genjidb Go-SQL database are provided as well.
