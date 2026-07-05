---
title: Canvas, Chat, Git, Forge, and VMs
section: features
order: 2
summary: See which secondary work surfaces are stable, experimental, or plugin-owned.
---

Spacewave can open more than file and documentation objects. Availability depends
on whether a surface is a normal Quickstart, an experimental Quickstart, or a
plugin-owned viewer.

## Canvas

Canvas is a normal visible Quickstart. It creates a Space, initializes UnixFS,
seeds a canvas demo, and opens the canvas viewer. Use it for visual planning and
object organization.

## Git

Git is a normal visible Quickstart. It creates a persistent Git repository
wizard, then opens the wizard so you can create or clone a repository. The app
has static viewers for Git repositories and worktrees.

## Chat

Chat has static channel and message viewers, but its Quickstart is experimental
in the current visibility policy. When available, it creates a Space with a chat
channel and opens that channel.

## Forge

Forge is experimental. Its Quickstart seeds a dashboard, cluster, sample work,
and a worker tied to the creating session. The app has static Forge viewers for
dashboards, clusters, jobs, tasks, workers, passes, and executions.

## V86 virtual machines

V86 is experimental and plugin-owned. The app can create V86 wizard objects, and
the Quickstart can copy a default image and create a VM object, but the VM object
viewer is not statically registered in the app shell. It requires the relevant
dynamic plugin viewer.
