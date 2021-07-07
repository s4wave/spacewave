# UnixFS

> Replicated verified / signed unixfs on block graph.

## Introduction

Unixfs implements a unix filesystem (inodes, directories, files) on top of the
block graph system (using "file" and "blob") with storage in world graph.
Includes FUSE filesystem mounting implementations.

Support for the "World" engine allows UnixFS filesystems to contain links to
other filesystems. This can create a "graph" of p2p filesystems and resources.
Non-filesystem objects can be represented as directories and files in FUSE.

## Known Issues

 - unixfs-fuse-direct: caching issue:
   - cd fuseroot
   - cp ../main.go ./
   - cat main.go # several times
   - echo "test" > main.go
   - cat main.go # shows old content due to caching
