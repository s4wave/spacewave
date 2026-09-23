---
title: Developer Start Here
section: start
order: 1
summary: Pick your path: a live app with the sync library, a plugin that runs in a Space, or a change to Spacewave itself.
---

There are three ways to build with Spacewave. Pick the one that matches what
you are making.

- **A live TypeScript application.** The `spacewave` npm package gives your own
  Node server and browser clients typed collections that update live. You do
  not need the Spacewave app. Start with [Build a Live
  Application](/docs/developers/sync/build-a-live-application).
- **A plugin that runs inside a Space.** A Space is a user's container for one
  project, and a plugin adds new kinds of items to it. Start with [Build a
  Plugin](/docs/developers/plugins/build-a-plugin).
- **A change to Spacewave itself.** Read the model below, then the pages under
  Objects, SDK and RPC, and Contributing.

## The model

A Space is a shared object: state that Spacewave syncs between devices and
people.
The body of a Space is a World, a Hydra database of typed objects. Hydra is
Spacewave's storage layer. Each object in a World has a key and a type. An
ObjectType is the registered code that turns an object key into a typed RPC
interface for one kind of object. A viewer is the UI component that displays it. A Quickstart
creates a new Space with starter content.

Every typed object needs an ObjectType, a resource that exposes its reads,
writes, and watch streams, and at least one viewer or command.

## Put code in the right layer

- The SDK and core packages hold protocol, storage, and resource behavior.
- The app packages hold product viewers, routes, Quickstarts, and commands.
- Plugin packages hold features that load from a manifest, the versioned
  package a plugin ships as. They register ObjectTypes, viewers, Quickstarts,
  or resources while they run.

Keep rules about stored data in a resource, an ObjectType, or a Space operation,
not in a route or a viewer.

## Add a new kind of object

1. Define or reuse the ObjectType and the World operation that creates the
   object.
2. Expose a typed resource for reads, writes, and watch streams.
3. Register a viewer for the ObjectType.
4. Add a wizard if users should create the object from inside a Space.
5. Add a Quickstart only if the object is useful as the first thing in a new
   Space.
6. Package it as a plugin if it should load on demand.

## Load data in React

Use the Resource SDK hooks for server state. `useResource` manages the lifetime
of a resource handle. `useStreamingResource` follows a watch RPC. Do not load
resource state with a raw `useEffect` and `useState`. See [Resource SDK and
Watch RPCs](/docs/developers/sdk/resource-sdk-and-watch-rpcs).

## Build and run the CLI

`bun run build:cli` builds the native CLI into `bin/spacewave`.
`bun run cli:local -- <command>` runs it against a state directory at
`./.spacewave`. For docs or viewer work, run focused tests before broad checks.
