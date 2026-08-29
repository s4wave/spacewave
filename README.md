<div align="center">
  <a href="https://spacewave.app" target="_blank" rel="noopener noreferrer">
    <img width="324" src="./doc/img/spacewave-github.png" alt="Spacewave">
  </a>

  <h3>Effortless self-hosted cloud with peer-to-peer sync</h3>

  <p>
    Open it in your browser: no account, no server, working offline in one click.
  </p>
</div>

--------

**Spacewave** runs [in your browser] with no servers: a private, encrypted workspace 
running entirely on your own machine: no account, no setup, no server to run. Syncs 
peer-to-peer between your devices and works offline.

As your work grows, the workspace becomes a Space: shared encrypted state for
files, notes, apps, layouts, devices, and workflows. Spaces are multiplayer:
invite people into one and work in it together, live.

You choose where your data lives and how it moves. Store it on your machines,
in your browser, on your own server, or in a cloud you pick, and sync over any
network your devices can reach. A paid account adds managed cloud storage and
relay, $8/month for 100 GB; the app works without one, and the cloud never
owns your work.

Make it your own with the same machinery the app is made of: a Go, TypeScript,
React, and WebAssembly stack where every app is local-first, multiplayer, and
encrypted by default.

Features:

- **Spaces**: One shared place for your files, notes, apps, devices, and workflows
- **Multiplayer**: Invite people into a Space and work in it together, live
- **Peer-to-Peer Sync**: Sync directly between your devices and keep working offline
- **Any Network**: Connect over WebRTC, WebSocket, LAN, or relay - whatever your devices can reach
- **Pluggable Storage**: Keep data on your machines, in your browser, or in any backend you choose
- **End-to-End Encrypted**: Storage, sync, and collaboration are private by default
- **Open-Source Full Stack**: Build your own apps and plugins with Go, TypeScript, React, and WebAssembly

**Plugin Ecosystem**: Extend a Space with plugins:

  - Files, databases, notes, and document collaboration
  - Chat and messaging
  - Terminal access and remote control of your devices
  - [SkiffOS] for building and managing Linux devices

[in your browser]: https://spacewave.app
[Spacewave App]: https://spacewave.app
[SkiffOS]: https://skiffos.com

## Getting Started

Where would you like to start?

- I want to use or self-host Spacewave: [Open Spacewave to get started]!
- I want to modify Spacewave or build my own plugins: see [running from source](#-running-from-source)
- I want to develop my own app with Spacewave Stack: see [Bldr]

[Open Spacewave to get started]: https://spacewave.app
[Bldr]: ./bldr

You can try Spacewave instantly in your web browser, just [click here].

[click here]: https://spacewave.app

## Running from source

To start the Spacewave app:

```bash
# Install dependencies
bun install
uv sync --frozen --all-groups

# Start the desktop app
bun run start:desktop

# Start the web app with GoScript browser Go plugins
bun run start:web

# Start the web app with standard Go/WASM browser Go plugins
bun run start:web:wasm
```

To run the test suite:

```bash
# All tests
bun run test

# Go tests only
bun run test:go

# Lint
bun run lint

# Typecheck
bun run typecheck
```

Spacewave uses [Protobuf](https://protobuf.dev/) for message encoding.

You should re-generate the protobufs after changing any `.proto` file:

```bash
# generate the protobufs
bun run gen
```

## Why Spacewave?

Traditional web-apps store and process data on servers and cloud infrastructure.
Developers write API calls to access the user data, and the frontend just
displays the response. Recent trends towards server-side rendering have
increased dependence on servers and the cloud even more.

This works great for static websites and services that require an internet
connection. But what if we want apps that work offline, are open-source, or use
features traditionally expensive to scale like multiplayer sync?

Imagine if every video game you played was rendered fully server-side. That game
would be way too laggy to play smoothly, right? This is how our modern web apps
are designed and built. But it doesn't have to be this way.

Modern web browsers come with WebAssembly, WebWorkers, ServiceWorkers,
SharedWorkers, IndexedDB, and WebRTC. These features enable building **fully
self-sufficient** apps running fully on the client side.

Spacewave utilizes these features to provide resources to apps with an
abstraction layer smoothing the differences between platforms. For example, an
app can allocate a SQL database and store it in Redis, BadgerDB, or IndexedDB,
all without changing a single line of code when switching backends.

We look forward to building a new generation of apps that are both open-source
and cloud-enabled, without requiring users to jump through hoops to self-host
and manage their own servers.

## Architecture

The goal is to build software that runs anywhere with any storage backend.

Spacewave runs the app logic in WebWorkers/SharedWorkers in the web browser and
as native processes on desktop. This brings the entire Go ecosystem to the
browser while enabling true [local-first] apps.

[local-first]: https://www.inkandswitch.com/local-first/

Components:

- **[Bifrost]** - Network over any transport
  - Cross-platform peer-to-peer communication
  - Encrypted transport protocols with stream multiplexing
  - Supports WebRTC and WebSocket in the web browser

- **[Hydra]** - Store data anywhere w/ p2p sync
  - Many supported data structures: SQL, K/V, GraphDB, ...
  - Pluggable storage backends: BadgerDB, Redis, S3, ...
  - Supports IndexedDB in the web browser

- **[Bldr]** - Build and run on any OS or browser
  - Build system and development environment
  - Hot reload and fast JS bundling with [esbuild] and [vite]
  - Cross-platform build and release

- **[GoScript]** - Compile Go to TypeScript for the web browser
  - Builds Go module packages into readable TypeScript modules
  - Shares Go algorithms, data structures, and runtime code without a second implementation
  - Powers Bldr's Go compiler mode for Spacewave web plugins and browser builds

- **[SkiffOS]** - Build and run on any device (w/ Linux)
  - Supports 40+ device types
  - Cross-compiles to target any architecture
  - Minimal with modular configuration

- **[Forge]** - Continuous integration and automation
  - Distributed job scheduler
  - CI/CD pipeline automation
  - Workflow orchestration

- **[Auth]** - Authentication methods and credential flows
  - Password and PEM-based authentication primitives
  - Shared auth building blocks for provider and session flows

- **[Identity]** - Identity and domain primitives
  - Identity records and domain-backed configuration
  - Supporting types for account and provider workflows

[Bldr]: ./bldr
[Bifrost]: ./net
[Hydra]: ./db
[Forge]: ./forge
[Auth]: ./auth
[Identity]: ./identity
[GoScript]: https://github.com/s4wave/goscript
[SkiffOS]: https://github.com/skiffos/skiffos
[esbuild]: https://esbuild.github.io/
[vite]: https://vite.dev

All components are designed to be used in multiple ways:

- As an application: each component has its own CLI
- As a library: all Go packages are documented as libraries
- As part of Spacewave: controller configuration with YAML/JSON

Spacewave can be extended with custom plugins or modifications to the client,
and custom apps can use the libraries to directly access the data stored in your
workspaces.

The current repository is a monorepo. The root app layer sits alongside the
component directories:

- `net/` - Bifrost
- `db/` - Hydra
- `bldr/` - Bldr
- `forge/` - Forge
- `auth/` - Auth
- `identity/` - Identity
- `app/`, `core/`, `sdk/`, `web/`, `cmd/` - the Spacewave application layer

## Space: The Final Frontier

Let's take a moment to look far into the future. We're sending the first people
to Mars. What OS are they using on their laptops and phones? What apps are they
using to share files, collaborate, and communicate?

The latency of a one-way message from Mars to Earth varies between 2 to 24
minutes. HTTPs does not work with this much latency, and even begins to break
with round-trip times over one second, let alone two minutes. Our existing
internet apps would not work in this environment.

Spacewave solves this problem with a local-first p2p architecture. Regardless of
internet latency or equipment failure, users can access their workspaces and
apps, without the need for cloud, servers, or on-call engineers.

Peer-to-peer networking works even when the internet doesn't.

## License

Spacewave is licensed under the permissive Apache-2.0 license.

All dependencies are [verified] to be equally permissive, one of:

- 0BSD
- Apache-2.0
- Apache-2.0 OR MIT
- BSD-2-Clause
- BSD-3-Clause
- CC0-1.0
- ISC
- MIT
- MPL-2.0

All of these licenses support commercial use, modification, and redistribution with attribution.

[verified]: https://github.com/s4wave/spacewave/actions/runs/24951306003/job/73061745956#step:8:1

## Support and Community

Spacewave is a community project to build the **most powerful** collaborative
workspace and **self-hosting** tool.

We welcome contributions in the form of GitHub issues and pull requests.

Please open a [GitHub issue] with any questions / issues / suggestions.

[GitHub issue]: https://github.com/s4wave/spacewave/issues/new

... or feel free to reach out via [Email] or [Discord]!

[Email]: mailto:oss@spacewave.app
[Discord]: https://discord.gg/KJutMESRsT
