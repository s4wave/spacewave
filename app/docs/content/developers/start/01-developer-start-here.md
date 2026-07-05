---
title: Developer Start Here
section: start
order: 1
summary: Start from Spaces, ObjectTypes, viewers, Quickstarts, plugins, and resource handles.
---

Spacewave development starts with the runtime object model. A Space is a
SharedObject whose body is a Hydra World. The World contains typed objects. Each
typed object needs an ObjectType owner, a resource contract, and one or more
viewers or commands.

## Build in the owning layer

- Use the SDK and core packages for protocol, storage, and resource semantics.
- Use app packages for product viewers, routes, Quickstarts, and command
  registration.
- Use plugin packages when a feature should load through a Manifest and register
  object types, viewers, Quickstarts, or resources dynamically.

Do not put durable state rules in a route or a viewer if a resource, ObjectType,
or Space operation owns them.

## The normal path

1. Define or reuse the ObjectType and world operation that creates the object.
2. Expose a typed resource handle for reads, writes, and watch streams.
3. Register an ObjectViewer for the object type.
4. Add an ObjectWizard when users should create the object from a Space.
5. Add a Quickstart only when the object is useful as a first-run Space.
6. Add plugin Manifest wiring when the feature should load dynamically.

## React data flow

Use the Resource SDK hooks for server state. `useResource` owns handle lifetimes.
`useStreamingResource` owns watch RPCs. Raw `useEffect` plus `useState` should
not own async data loading for resource state.

## CLI and verification

Use `bun run build:cli` to build the native CLI into `bin/spacewave`. Use
`bun run cli:local -- <command>` to run the local CLI against `./.spacewave`.
For docs or viewer work, run focused tests before broad checks.
