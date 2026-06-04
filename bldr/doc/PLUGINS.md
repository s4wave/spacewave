# Plugins

- Entrypoint: the "main process" of this system - initial starting point.
  - Requires restarting the program fully to update the Entrypoint.
  - Rarely updated: all programs can use the same binary executable.
  - Stores a configuration for the initial sub-plugin to load.
- Plugin: loaded as a sub-process of the Entrypoint.
  - Loading a new version: unload the current, load the new.

## Go

Go plugin:

- Compiled with a list of Go packages
- Auto-register Controller factories from those packages to the Bus
- Executes a configured ConfigSet on the bus on startup
- If no "entrypoint" is defined the plugin just contains static files.
- The plugin compiler includes controllers for loading web pkgs, serving assets, and more.

### GoScript browser mode

`COMPILER_MODE_GOSCRIPT` is the browser-only Go plugin compiler mode for the
`web/js/wasm` platform. Bldr compiles the generated plugin module with GoScript,
using the release/dev build tags, source-local `gs` overrides, full dependency
graph output, and protobuf TypeScript binding.

After GoScript writes the TypeScript package tree, Bldr builds
`plugin-goscript-entrypoint.ts` through the Bldr-owned Rolldown/Oxc wrapper
path. The generated package tree stays under `dist/@goscript` for inspection;
the served entrypoint is the bundled `.mjs` output.

The wrapper preserves generated-tree `@goscript/...` imports, relative
JavaScript-to-TypeScript sibling resolution, Bldr SDK aliases, `@go/...`
vendor/local module imports, the `node:events` browser shim, source-map policy,
minification policy, and dependency input accounting. Each wrapper build writes
`plugin-goscript-bundle-report.json` in the build work directory with output
bytes, best-gzip bytes, minify/sourcemap policy, and Rolldown dependency inputs.
That report is build-private and is not emitted into the plugin manifest output.

## Js

Js plugin:

- Compiled with a list of bundles of .js or .ts files (Vite or Esbuild inputs)
  - output path defaults to `path/to/foo.ts => path/to/foo.js`
  - same bundle => files are split to share code as much as possible with import() and esm
- The "entrypoint" .js file is expected to export `main` which accepts a `PluginAPI` object and returns a Promise<void>
  - the function name to call is configurable
  - the promise should not resolve until the program is done executing
- If no "entrypoint" is defined the plugin just contains static files.
