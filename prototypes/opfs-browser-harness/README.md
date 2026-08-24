# GoScript OPFS browser harness

This harness compiles a small Go package that builds a Spacewave world with the
OPFS volume and world engine. The package creates two world objects and a graph
edge, syncs the world, closes it, reopens it, and reads the objects and edge
back. The browser entry reports the result in `window.__opfsResult` and in the
page.

Run the browser proof in Chromium and WebKit from the repository root:

```bash
go test -v ./prototypes/opfs-browser-harness -run TestOpfsBrowserHarness -count=1
```

The test compiles the package with GoScript, bundles the generated modules with
Rolldown, serves `index.html` over localhost, and runs the page in each engine.
Each engine launches through a persistent profile directory because WebKit
exposes OPFS only in a persistent session.
