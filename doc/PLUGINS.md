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

## Js

Js plugin:

- Compiled with a list of bundles of .js or .ts files (Vite or Esbuild inputs)
  - output path defaults to `path/to/foo.ts => path/to/foo.js`
  - same bundle => files are split to share code as much as possible with import() and esm
- The "entrypoint" .js file is expected to export `main` which accepts a `PluginAPI` object and returns a Promise<void>
  - the function name to call is configurable
  - the promise should not resolve until the program is done executing
- If no "entrypoint" is defined the plugin just contains static files.
- The TypeScript code is responsible for registering handlers for http requests and/or accessing assets.

