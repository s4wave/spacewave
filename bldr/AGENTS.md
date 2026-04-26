## Bldr-Specific Rules

- DO NOT assume `bldr setup` needs to be run - it runs automatically when bldr starts for almost any operation
- DO NOT manually copy files to `.bldr/src/` or sync source files there. `.bldr/src/` is managed by `bldr setup` and regenerated automatically. Edit source files in their original locations only.

## Code Signing

Bldr's Go compiler (`util/gocompiler`) signs every produced binary when
platform-appropriate signing env vars are set. Hook runs after `go build`,
before any wasm post-processing. No-op when the identity env is unset.

### macOS

| Env var | Meaning |
|---|---|
| `BLDR_MACOS_SIGN_IDENTITY` | codesign identity. Example: `Developer ID Application: Aperture Robotics LLC (6YCJAUQGQ6)`. Unset = skip signing. |
| `BLDR_MACOS_SIGN_ENTITLEMENTS` | Path to entitlements plist. Optional. |
| `BLDR_MACOS_SIGN_OPTIONS` | Comma-separated codesign `--options` values. Defaults to `runtime`. |

When set and GOOS=darwin, bldr shells out to:
```
codesign --force --sign "$IDENTITY" --options "$OPTIONS" [--entitlements "$ENTS"] <binary>
codesign --verify --strict <binary>
```
Either non-zero exit fails the build.

### Windows

| Env var | Meaning |
|---|---|
| `BLDR_WINDOWS_SIGN_PROFILE` | Trusted Signing certificate profile name. Unset = skip signing. |
| `BLDR_WINDOWS_SIGN_ACCOUNT` | Trusted Signing signing-account name. Required when profile is set. |
| `BLDR_WINDOWS_SIGN_ENDPOINT` | Regional endpoint URL. Defaults to `https://wus.codesigning.azure.net/` when unset. |
| `BLDR_WINDOWS_SIGN_DESCRIPTION` | Authenticode signature description. Defaults to `Spacewave`. |

When set and GOOS=windows, bldr shells out to:
```
pwsh -NoProfile -NonInteractive -Command "Invoke-TrustedSigning -Endpoint $env:BLDR_SIGN_ENDPOINT -CodeSigningAccountName $env:BLDR_SIGN_ACCOUNT -CertificateProfileName $env:BLDR_SIGN_PROFILE -Files $env:BLDR_SIGN_FILE -Description $env:BLDR_SIGN_DESCRIPTION -FileDigest SHA256 -TimestampRfc3161 'http://timestamp.acs.microsoft.com' -TimestampDigest SHA256"
```
Requires the `TrustedSigning` PowerShell module
(`Install-Module -Name TrustedSigning`) and a prior `az login` (or
`azure/login@v3` in CI) so that `DefaultAzureCredential` can authenticate
via `AzureCliCredential`. Non-zero exit fails the build.

## File Logging

Bldr supports file-based logging via the `--log-file` flag and `BLDR_LOG_FILE`
environment variable. Implementation is in `util/logfile/`.

```bash
# Explicit file logging
bldr --log-file 'level=DEBUG;format=json;path=.bldr/logs/{ts}.log' start web

# Via environment variable
BLDR_LOG_FILE='level=WARN;path=/tmp/bldr-warn.log' bldr start web

# Short form (path only, defaults to level=DEBUG;format=text)
bldr --log-file '.bldr/logs/{ts}.log' start web

# Disable auto-logging in dev mode
BLDR_LOG_FILE=none bldr start web
```

In dev mode (`--build-type dev`), file logging is auto-enabled with
`level=DEBUG;path=.bldr/logs/{ts}.log`. Log files are created under
`.bldr/logs/` with session-stamped filenames. No auto-cleanup or rotation.

## Dist Sources (Embedded TypeScript Files)

When adding new TypeScript files that need to be bundled for the Electron or browser entrypoints, you must add them to the `//go:embed` directives in `dist.go`.

The `DistSources` embed.FS contains TypeScript sources used by esbuild during the build process. If a new `.pb.ts` file or other TypeScript module is imported by files in `web/electron/` or `web/entrypoint/`, it must be explicitly embedded.

Without this, esbuild will fail with "Could not resolve" errors when building the Electron or browser bundles.

## Proto Imports

Proto files use Go-style import paths based on Go module names (from `go.mod`).

**Within this project:**

This project's module is `github.com/s4wave/spacewave`. Local proto files reference each other using the full Go module path:

```protobuf
// In sdk/session/session.proto
import "github.com/s4wave/spacewave/core/session/session.proto";
import "github.com/s4wave/spacewave/core/sobject/sobject.proto";
```

**From external Go modules:**

```protobuf
import "github.com/aperturerobotics/controllerbus/bus/bus.proto";
import "github.com/aperturerobotics/starpc/srpc/srpc.proto";
```

**Package naming conventions:**

- `sdk/` files use the full `s4wave.` prefix (e.g., `package s4wave.space;`)
- `core/` files use shortened package names without the prefix (e.g., `package space.world;`)
- When referencing types from `core/` packages in `sdk/` files, use a leading `.` for fully-qualified references (e.g., `.space.world.WorldContents`)

## Test Structure

Bldr has four test tiers. Choose the narrowest tier that covers the behavior.

### Unit tests (`*.test.ts`)

Run with `vitest run` (happy-dom environment). Co-locate with the module under
test (e.g. `web/bldr/sab-ring-stream.test.ts` beside `sab-ring-stream.ts`).
Use for pure logic, data structures, ring buffers, parsers, protocol helpers.
No real browser APIs, no network, no filesystem.

### Browser tests (`*.browser.test.ts`, `*.e2e.test.ts`)

Run with vitest browser mode (Playwright provider, headless Chromium). Use when
the test needs real browser APIs (SharedArrayBuffer, Atomics, OPFS,
BroadcastChannel, Web Locks, service workers). Co-locate with the module.
Cross-origin isolation headers are applied automatically via the vitest config
plugin.

### E2E tests (`e2e/*.spec.ts`)

Run with `bun run test:e2e` (Playwright directly, not vitest). The Playwright
config at `e2e/playwright.config.ts` starts the dev server via
`bun run start:web:wasm` and waits for it. Use for validating the full
application lifecycle: page loads, WASM boots, plugins render, no console
errors. Add new specs here when testing cross-cutting behavior that requires the
full bldr dev server running (Go WASM runtime + Vite + plugin compilation).

```bash
bun run test:e2e           # headless
bun run test:e2e:headed    # visible browser
bun run test:e2e:ui        # Playwright inspector
```

### Release E2E tests (`web/entrypoint/browser/*.e2e.spec.ts`)

Run with `bun run test:release:web` which first builds a release web bundle
then tests against the static output. Use for validating release builds
specifically (service worker registration, minified bundles, static serving).

### Go tests (`*_test.go`)

Run with `go test ./...`. Standard Go test files co-located with their
packages. Use for Go-side logic: compilers, manifests, platforms, storage,
bundler internals, RPC server plumbing.

### Prototypes (`prototypes/`)

Playwright specs under `prototypes/` are research experiments with their own
`playwright.config.ts` and static HTML fixtures. They are excluded from all
vitest projects and from `bun run test:e2e`. Do not add production tests here.

### When to add which test

- New utility function or data structure: unit test (`*.test.ts`)
- New browser API integration (SAB, OPFS, Web Locks): browser test
  (`*.browser.test.ts`)
- New user-visible feature or startup path: e2e test (`e2e/*.spec.ts`)
- New Go package or compiler behavior: Go test (`*_test.go`)

## Linting and Typechecking

After making code changes, verify they compile correctly:

```
bun run typecheck
bun run lint
go build ./...
```

## Rebuilding .bldr

If you encounter issues with `.bldr` (stale exports, module resolution errors, etc.), rebuild it with:

```
bun run setup
```

This regenerates the `.bldr/src` directory from source.
