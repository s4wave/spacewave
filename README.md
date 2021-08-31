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
 - [rGraphQL]: real-time streaming GraphQL requests w/ lazy-loading.
 
[ControllerBus]: https://github.com/aperturerobotics/controllerbus
[Bifrost]: https://github.com/aperturerobotics/bifrost
[Hydra]: https://github.com/aperturerobotics/hydra
[Identity]: https://github.com/aperturerobotics/identity
[Auth]: https://github.com/aperturerobotics/auth
[rGraphQL]: https://github.com/aperturerobotics/magellan

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

## Developing

You need the following installed:

 - [Go](https://golang.org) >= 1.16
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

## Distributing

To bundle to all of the configured targets:

```bash
# Bundle to all targets & store as a Hydra manifest.
yarn run bundle
# alternatively use the cli:
bldr bundle
```

## License

Copyright 2021 Aperture Robotics LLC.
