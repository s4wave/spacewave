# Fibonacci Heap

> Fibonacci Heap implementation on top of db.Db.

## Introduction

This Fibbonaci Heap implementation is designed for:

 - Fast priority queuing on top of a database.
 - Minimal computation for insertion, find-minimum, merge heaps.
 - Minimal accesses to storage.
 
Two pieces of data are stored in-band with pointers: Key (string), Weight (integer).

Pointers are implemented with string keys.

## Algorithm Details

General stats:

 - Find-Minimum: O(1)
 - Insert: O(1)
 - Delete element: O(log(n))
 - Merge Heaps: O(1)

This particular implementation focuses on consistency with the non-transactional
operations to the K/V Database.
