# Bldr

> Pluggable cross-platform UI.

## Introduction

Bldr uses the Aperture stack to build modular UIs and applications.

Plugin bundles are executed as native processes or WebAssembly WebWorkers.

The compiler can target multiple deployment strategies:

- Daemon: running as a native Go process.
- CLI: client interfaces on the command line.
- Web Browser: UI, with esbuild and web technologies.
- Desktop App: bundled web view and web-powered UIs.
- Firmware: embedded firmware (such as with TinyGo).

Each UI and application logic module / library is built independently and can
use any linting, compilation, and frontend technologies. The primary tools used
here are React, Snowpack, and esbuild for lightning-fast hot-reloading UIs.

## Developing

You need the following installed:

 - [Go](https://golang.org) >= 1.16
 - If using UI: [Node](https://nodejs.org) >= v16
 - Yarn `npm install yarn`

Initial setup (if using web UIs):

```bash
# download deps
yarn
```

To start the Browser/WebWorker version of the app Sandbox (editor):

```
# build go wasm
yarn build:wasm
# start snowpack app
yarn start
```

## Distributing

Bldr contains tools for "Whitelabel" branded apps.

To bundle to all the possible targets:

```bash
# Bundle to all targets & store as a Hydra manifest.
yarn run bundle
# alternatively use the cli:
bldr bundle
```

## License

Copyright 2021 Aperture Robotics LLC.
