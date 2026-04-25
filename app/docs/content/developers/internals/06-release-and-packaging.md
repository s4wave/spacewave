---
title: Release and Packaging
section: internals
order: 6
summary: Release pipeline, signed artifacts, and packaging targets.
---

## Overview

Spacewave builds and packages through bldr, a build orchestration system that produces artifacts for browser, desktop, and CLI targets. The build pipeline compiles Go to WebAssembly, bundles TypeScript with Vite, generates plugin manifests, and prerenders static pages. Release builds are triggered via npm scripts and produce deployable artifacts.

## Build Targets

The `build` section of the bldr project config defines named targets that combine manifests with platform IDs:

| Script | Target | Output |
|--------|--------|--------|
| `bun run build` | app (debug) | Development build |
| `bun run build:release` | release | All platforms |
| `bun run build:release:web` | release-web | Browser WASM bundle |
| `bun run build:release:desktop` | release-desktop | Native desktop binary |
| `bun run build:cli` | CLI | `bin/spacewave-cli` |

Debug builds skip minification for faster iteration. Release builds produce optimized, content-addressed artifacts.

## Build Pipeline

A release build follows this sequence:

1. **Go compilation** - Go source is compiled to WASM (for browser) or native binary (for desktop). The WASM binary is placed in the dist bundle.
2. **Plugin manifests** - Each plugin declared in the bldr config is compiled. JS plugins are bundled with esbuild. Go plugins are compiled to WASM. The manifest is a content-addressed block containing compiled code and metadata.
3. **Vite build** - TypeScript frontend code is bundled with Vite, producing hashed CSS, JavaScript, and asset files.
4. **Prerender** - Static HTML pages are generated from React components using the Vite-built CSS and the bldr dist manifest (see [Prerender and Public Web](/docs/developers/internals/prerender-and-public-web)).
5. **Artifact assembly** - All outputs are collected into the dist directory with a `manifest.json` describing entrypoints.

## Development Workflow

During development, `bun run start:web` starts the bldr dev server with hot reload. Changes to TypeScript files trigger Vite HMR. Changes to Go files trigger WASM recompilation. The `rev` field on manifest entries in the bldr config must be incremented to force a manifest rebuild; `bun fecheck` handles this automatically for frontend changes.

## Frontend Checks

Before showing frontend changes to a user, run:

```bash
bun fecheck
```

This increments the manifest `rev` field, runs TypeScript type checking (`bun run tsgo --noEmit`), and validates the production Vite build including Tailwind utility verification.

## Testing

```bash
bun testcheck       # Abbreviated summary of all tests
bun run test        # Full verbose output
bun run test:js     # Vitest unit tests + typecheck
bun run test:go     # Go tests (skipping E2E)
```

Browser E2E tests run the full WASM stack in Playwright. They exercise the real runtime, not mocks.

## CLI Packaging

The CLI is built with `bun run build:cli`, which compiles the Go binary and copies it to `bin/spacewave-cli`. The CLI shares the same Go codebase as the WASM runtime but runs as a native process with direct filesystem access.

## Next Steps

- [Project Configuration](/docs/developers/platform/project-configuration) for the bldr project config format.
- [Plugin Lifecycle](/docs/developers/platform/plugin-lifecycle) for how plugins are built and distributed.
