# Bldr

> Pluggable cross-platform UI.

## Introduction

Bldr uses the Aperture stack to build modular UIs and applications.

The compiler can target multiple deployment strategies:

- Daemon: running as a native Go process.
- CLI: client interfaces on the command line.
- Web Browser: UI, with esbuild and web technologies.
- Desktop App: bundled web view and web-powered UIs.
- Firmware: embedded firmware (such as with TinyGo).

Each UI and application logic module / library is built independently and can
use varying linting, compilation, and frontend technologies.

## Developing

You need the following installed:

 - [Go](https://golang.org) >= 1.15
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

To bundle to a desktop app:

```bash
# Bundle to web app.
yarn run build
# Bundle to desktop app.
yarn run dist:electron
```

## License

Copyright 2021 Aperture Robotics LLC.
