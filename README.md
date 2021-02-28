# Forge

> Cross-language build system orchestrator with distributed builds.

## Introduction

Forge is a system for defining graphs of operations to perform on data to
produce desired build outputs. Each build step is expected to consume inputs and
create one or more outputs. Build steps form a Build Graph and a list of tasks
to perform to produce a given target. The build steps can then optionally be
distributed over a peer-to-peer build cluster.

Forge is used to implement tools which automatically archive/backup sources,
assemble together applications and targets, and audit binaries against provided
sources. It can also be used as a generic peer-to-peer data pipeline.

Uses the Hydra p2p storage and sync engine with the Anchor blockchain.

