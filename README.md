# Hydra

> Modular peer-to-peer storage with block-graph data structures.

## Introduction

Hydra is a peer-to-peer object and block store:

 - *Storage agnostic*: manage multiple local and remote storage volumes.
 - *Rapid transfers*: leverage optimized data transfer paths between volumes.
 - *Replication*: bucket policies implement data replication behaviors.
 - *Encryption*: transformations can encrypt data at rest.
 - *Persistent identity*: each volume has an identity independent from the host.
 - *Cross platform*: support every platform, including the web browser.
 - *Modular components*: pluggable implementations for flexible configuration.

The general purpose of Hydra is to build a resilient storage engine for linked
DAG object / block structures, capable of communicating with arbitrary storage
backends and replicating data to ensure redundancy.

An additional constraint is that all nodes in the network do not trust each
other. Data must be verified when performing or completing transfers and stores.
Typically this verification occurs by hashing the data and comparing the hash to
the expected value. As objects are stored with their hash as their ID (the
content ID approach) it's then impossible to read inconsistent data if we
validate the hash at storage time or read time (depending on the expected
consistency of the store). With this property, Hyra is similar to immutable
stores such as Bittorrent.

## Design Overview

Terminology and overview:

 - Block: the smallest data primitive, hashed and identified by content-ID.
 - Object: decoded block or set of blocks with pointers forming a DAG.
 - Volume: storage space local or remote managed by a controller.
 - Bucket: collection of blocks, with attached data management policies.
 - MQueue: FIFO message queue, typically attached to a reconciler.
 - Reconciler: controller which processes bucket events localized to a Volume.
 - Lookup: operation against a bucket over many volumes with a controller.
 - Blob: large data split into deterministic chunks with Rabin fingerprinting.
 - File: a collection of written Ranges composed of Blobs of data.
 
Hydra is built on the ControllerBus framework, which defines the Config,
Controller, Directive structures and behaviors. All components are implemented
as controllers, and have associated factories.

An EntityGraph controller is provided. EntityGraph exposes the internal state
representation of Hydra and other systems to visualizers and instrumentation via
a graph-based inter-connected entity model.

### Block

A "Block" is a binary blob of arbitrary size. The underlying storage engine
works in terms of the basic Block primitive. Blocks can be encoded using any
encoding and/or compression algorithms. Blocks are hashed at storage time and
that hash is used as a content ID for the Block.

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

### Naming

Hydra is a content-identified data store, using unique properties of data
(hashes) to uniquely identify and name objects. No attempt is made to map
between human readable filenames and objects. The "unixfs" component implements
a FUSE-mountable unix filesystem on top of the Block store.

### Data consistency

Hydra implements a Write-Only-Read-Many model (WORM): a Block, once created, is
immutable. Blocks are referenced by their hash, using content identifiers. This
property allows the data to be efficiently transported through p2p networks such
as BitTorrent. Once a Block is created, it cannot be guaranteed that the Block
will ever cease to exist in the network, although policies can be adjusted to
trigger garbage collection of data.

## Volume

A "Volume" is storage space in the form of memory, disk, or networked stores.
Each Volume is assigned a generated public/private keypair. Each Volume's
storage capacity, transfer rate, node association, identity, active transfers,
and other properties are tracked by the "Volume Controller." Requests to
manipulate data are interpreted and executed by the volume controller.

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

## Reconcilers

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

### Replication and synchronization

Replication reconcilers control how and in what order/priority a node will
attempt to replicate (copy) data between Volumes. Replication directives specify
where data should be stored in precise or general terms to be interpreted by
replication reconcilers in the network. Controllers may communicate to form a
local consensus of data placement and short-term planned data transfers to drive
the general network equilibrium towards the desired goal state.

### Fault detection

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

### Example: volume startup sequence

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

## Concurrent Message Queue

A Message Queue is a store-backed, FIFO, at-least-once delivery, concurrent
reader and writer safe structure. It can be implemented with various algorithms
given the underlying store implementation, an example of a particularly safe
implementation being a transactional key-value store.

## Code Organization

These are the types of implemented data structures:

 - block: content-ID reference graph
   - blob: split a blob of data into multiple blocks
   - file: copy-on-write file implementation using blobs
   - git: stores a git repository with go-git
   - iavl: avl tree, implements kvtx
   - object: reference a block in a different bucket or with different
    transformation parameters
 - dex: data exchange protocols
 - heap: common interface for all heaps
   - heaptest: test for all heap stores
 - kvtx: transaction-based key/value store
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
 - volume: management of a storage backend
 
Each of the top-level directories contains a declaration of a data structure
interface, with higher-level data structures implemented on top of the declared
data structure in sub-directories.

## SQL Implementation

The MySQL-compatible implementation under `sql/mysql` uses the `go-mysql-server`
project mapped to a block DAG data structure. Msgpack is used to encode field
type information, and Blobs are used to chunk & store larger values.
