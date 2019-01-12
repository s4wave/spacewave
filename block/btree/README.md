# B Tree

> B tree implemented on top of an objstore.

## Introduction

This B tree implementation is designed for:

 - Key-value store on top of objstore databases.
 - Minimal computation for insertion, read, delete.
 - Minimal accesses to storage.
 - Deterministic changes to allow easy validation.
 
## Algorithm Details

General stats:

 - Find-Minimum: O(1)
 - Insert: O(1)
 - Delete element: O(log(n))
 - Merge Heaps: O(1)

This particular implementation focuses on consistency with the non-transactional operations to the K/V Database.

## Background

Why B Trees?

 - keeps keys in sorted order for sequential traversing
 - uses a hierarchical index to minimize the number of disk reads
 - uses partially full blocks to speed insertions and deletions
 - keeps the index balanced with a recursive algorithm

Some implementation gotchas:

 - when changing a node, need to also update the parents
 - keep dict of dirty nodes to write later
 
Objects:
 
 - BTree: keeps runtime references to root, rootref, rootnod. Mutex for write.
 - Root: stored object containing length + pointer to root node.
 - Node: node in the data structure.

A BTree can be mutable cloned in O(1), as every change to a BTree creates a new root object.

Nodes keep a pointer to their parent node. A dictionary of "dirty" nodes is kept for each operation.

 - Nodes are added to the dirty set when changed.
 - The flush operation updates the parents recursively, pushing them into the dirty set.
 - Priority queue for node flush ordered by depth in tree.
 - Flushing a node involves: write to storage, update parent reference.
