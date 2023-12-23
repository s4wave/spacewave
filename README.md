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
- **Web Browser**: using WebAssembly and WebWorkers.

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
 - **deploy**: pushes plugins to a plugin registry
 - **dist**: bundles distribution archives (release tarballs)

The bldr developer tool uses Go and **esbuild** to bundle Go, JavaScript,
TypeScript, and other assets into **Plugins**.

When a **Plugin** is loaded, its startup **ConfigSet** is applied, executing any
configured startup controllers.

**Plugins** can communicate with the host and each other via RPC services.

The **LoadPlugin** directive instructs the plugin host to load a plugin by ID.

### Web

The **web** layer for bldr adds additional concepts:

 - **WebDocument**: browser page, tab, or Electron BrowserWindow.
 - **WebView**: location in the WebDocument where Go can load components.
 - **WebRuntime**: interface to access the Go runtime from JavaScript.

It uses the following browser mechanics:

 - **BroadcastChannel**: communications channel between two Js components.
 - **SharedWorker**: parallel background worker shared between all tabs.
 - **ServiceWorker**: intercepts HTTP requests and forwards to Go runtime.

When running as a native application (desktop, electron) the Go process is the
initial entrypoint to the application, and will start the WebRuntime as a
sub-process. For example: extracting & starting the Electron redistributable.

The Web frontend communicates with the Go backend via [RPC streams]. The
frontend and backend can be located in the same browser, as a native process
bundled with an Electron app, or separated into client/server.

[RPC streams]: https://github.com/aperturerobotics/starpc

The **ServiceWorker** intercepts HTTP requests to the `/b/` and `/p/` paths.
Plugins control the URL space below `/p/{plugin-id}/` and can serve any Go HTTP
handler at that path, including static assets bundled with the plugin.

### Esbuild

The plugin compiler scans Go code for comment directives, ex:

```go
// Entrypoint is the component entrypoint.
//
//bldr:esbuild root.tsx
var Entrypoint bldr_esbuild.EsbuildOutput
```

The available comment directives are documented here:

### `bldr:asset`

```go
// AppFavicon is the favicon .ico asset.
//
//bldr:asset favicon.ico favicon.ico
var AppFavicon string
```

Recursively copies a file and/or directory to the asset filesystem. Paths are
relative to the Go file containing the comment. The resulting URL is stored in
the variable associated with the comment.

### `bldr:asset:href`

```go
// AppFaviconHref is the URL to the .ico icon asset.
//
//bldr:asset:href favicon.ico
var AppFaviconHref string
```

Determines the URL to the given path relative to the assets filesystem root.
Conceptually similar to `path.Join(assetsFs, givenPath)`. The resulting URL is
stored in the variable associated with the comment.

### `bldr:esbuild`

```go
//bldr:esbuild --any-esbuild-flag component.tsx
var Component bldr_esbuild.EsbuildOutput
```

Uses Esbuild to bundle the contents of the given entrypoint to the plugin asset
filesystem. CLI arguments for esbuild are accepted in the comment. Multiple
lines with the bldr:esbuild prefix are joined into a single esbuild command.
Stores the URL to the root javascript and CSS file in the variable associated
with the comment.

An optional flag is the `--bundle-id=default` flag. All bldr:esbuild directives
with the same bundle ID will be combined together into a single esbuild bundle
request, with one esbuild entrypoint per bldr:esbuild directive. If you want to
have a separate bundle, specify a different `--bundle-id=value` in the comment.

Flags can also be specified in the plugin compiler config with "esbuildFlags".

## Build Tags

bldr will set the following build tags:

 - `bldr_analyze`: set while analyzing the code for factories
 - `build_type_dev`: set during development build
 - `build_type_release`: set during release build

## Developing

You need the following tools installed:

 - [Go](https://golang.org) >= 1.21
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
# start desktop application
yarn start:desktop
```

Note: in Chromium: to view the SharedWorker developer tools:

 - Open chrome://inspect
 - Click "inspect" on the SharedWorker - usually named `bldr:default**

### VSCode Debugging

To debug the Electron `main` process:

- Press Command + Shift + P
- Select "Debug: Attach to Node Process"
- Select the Electron instance.

To debug the Electron `renderer` process:

- Copy .bldr/src/.vscode/launch.json to `.vscode/launch.json`
- Select "Run and Debug" in the left bar.
- Click the green play button for "Debug Electron Renderer Process"

This is only enabled in Debug builds for Electron, where the plugin compiler
will automatically configure the web plugin to start Electron with: 

- `--inspect=5858` for the main process
- `--remote-debugging-port=9222` for the renderer

This is enabled by the plugin compiler for Electron in debug builds only.

## License

Copyright 2018-2023 Aperture Robotics, LLC.
