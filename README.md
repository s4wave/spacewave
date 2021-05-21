# Hydra

> Modular peer-to-peer storage with block-graph data structures.

## Introduction

Hydra is a modular storage engine designed to connect any data structure to any
data store with peer-to-peer synchronization and real-time queries.

It's used as the underlying storage engine for other Aperture projects.

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
 - heap: common interface for all heaps
   - heaptest: test for all heap stores
 - sql: contains all SQL implementations
   - genji: genjidb (based on kvtx)
   - mysql: mysql-compatible protocol (based on go-mysql-server)
 
Each of the top-level directories contains a declaration of a data structure
interface, with higher-level data structures implemented on top of the declared
data structure in sub-directories.

## SQL Implementation

The MySQL-compatible implementation under `sql/mysql` uses the `go-mysql-server`
project mapped to a block DAG data structure. 
