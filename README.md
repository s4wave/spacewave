![Bldr](./doc/img/bldr-logo.png)

## Introduction

**Bldr** leverages the Aperture Stack to build modular peer-to-peer applications
with real-time user interfaces that run anywhere. Deploys hot-loaded plugins via
the p2p network.

The bundler can deploy to many target environments:

- **CLI**: client interfaces on the command line.
- **Daemon/Cloud**: running as a native Go process.
- **Desktop**: supports Electron and/or bundled web views.
- **Firmware**: embedded devices (using TinyGo).
- **Mobile**: using the gomobile tool.
- **Web Browser**: using WebAssembly (or GopherJS) and WebWorkers.

Uses the Aperture Robotics stack to implement a full peer-to-peer application:

 - [ControllerBus]: communicating controllers w/ declarative config.
 - [Bifrost]: p2p communications + pub-sub with pluggable transports.
 - [Hydra]: storage engine with advanced p2p block-graph structures.
 - [Identity] and [Auth]: public-key identity and key derivation.
 - [staRPC]: bi-directional streaming RPCs between TypeScript and Go.
 - [rGraphQL]: live-updating streaming GraphQL requests w/ lazy-loading.
 
[ControllerBus]: https://github.com/aperturerobotics/controllerbus
[Bifrost]: https://github.com/aperturerobotics/bifrost
[Hydra]: https://github.com/aperturerobotics/hydra
[Identity]: https://github.com/aperturerobotics/identity
[Auth]: https://github.com/aperturerobotics/auth
[staRPC]: https://github.com/aperturerobotics/starpc
[rGraphQL]: https://github.com/rgraphql/magellan

Supports advanced data structures (even in the web browser) including:

- **Blob**: split a large piece of data into deterministic chunks.
- **File**: collection of written Ranges composed of Blobs of data.
- **Git**: code revision tracking engine with go-git.
- **Graph**: graph database w/ quads: `<subject, predicate, object, value>`
- **Kvtx**: transactional key/value store (i.e. AVL tree).
- **Sql**: SQL data store backed by GenjiDB or go-mysql-server.
- **UnixFS**: directories, files, permissions, FUSE mounts.
- **World**: key/value store coupled with a graph database + changelog. 

Each UI and application module is bundled separately and can use any linting,
compilation, and frontend approaches.

## Overview

Main concepts in the development workflow:

 - **Entrypoint**: manages executing plugins and starting the initial plugin.
 - **Plugin**: contains **controller** factories and a startup **ConfigSet**.
 - **Controller**: goroutine managing a portion of the application logic.
 - **ConfigSet**: list of controllers to start with configuration objects.

The bldr developer tool has the following major command categories:

 - **start**: starts applications in development mode
 - **deploy**: pushes plugins to target environments
 - **bundle**: bundles installation archives (release tarballs)

The bldr developer tool uses the Go compiler to build Go code, and **esbuild**
to bundle JavaScript, TypeScript, and other web assets.

## Developing

You need the following tools installed:

 - [Go](https://golang.org) >= 1.18
 - If using UI: [Node](https://nodejs.org)
 - Yarn `npm install -g yarn`

Initial setup (if using web UIs):

```bash
# download deps
yarn
```

To start the application for development:

```
# start web application
yarn start:web
# start electron application
yarn start:electron
```

Note: in Chromium: to view the SharedWorker developer tools:

 - Open chrome://inspect
 - Click "inspect" on the SharedWorker - usually named `bldr:default`

## License

Copyright 2018-2022 Aperture Robotics, LLC.
