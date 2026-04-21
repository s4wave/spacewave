# World

> Key/value store combined with a Graph database.

## Introduction

World objects can point to any Block structure, including: Key/value stores,
MySQL DBs, Git repos, Hydra Volumes, Unix filesystems, and other Worlds.

The World has an attached changelog which can be used to efficiently wait for
relevant changes to the watched objects. The [ControlLoop] utility implements a
loop which waits for changes to an object before calling a callback function.

[ControlLoop]: ./control

World is a series of interfaces with multiple available implementations, but is
primarily implemented as a forkable [Block graph] with Protobuf objects. Other
implementations might be read-only and/or watch real-world object states.

[Block graph]: ./block/world.proto
