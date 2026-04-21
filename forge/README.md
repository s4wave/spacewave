# Forge

> Cross-language task orchestrator with multi-pass distributed execution.

## Introduction

Forge is a cross-platform distributed Job pipeline system with p2p workers.

Forge has a wide variety of applications from build pipelines to automated
responses to real-world conditions.

[Hydra] defines the [World] structure as a key/value store with a Cayley graph.

The objects in the graph database can point to any other block-graph structure:
for example: Key/value stores, MySQL Databases, Git repos, even nested Worlds.

[Hydra]: https://github.com/s4wave/spacewave/db
[World]: https://github.com/s4wave/spacewave/db/tree/master/world

Forge adds **Jobs** with **Tasks** executed by **Workers** in **Clusters**. All
aspects are managed and exposed as World objects with links between. Multiple
Clusters can operate on a single Job at a given time, as each Task execution is
assigned to a single Worker from one of the clusters.

Each **Task** has a **Target** and one or more **Pass** as well as **Inputs**
and **Outputs**. Each Pass is an attempt to run an **Execution** of the task.
When the input values or the Target changes, a new Pass is created. Each Pass
can have multiple replicas to cross-check the output of multiple Workers.

## Library

The [lib](./lib) subdir contains various targets available in the core library.
