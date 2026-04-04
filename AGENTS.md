## Bldr-Specific Rules

- DO NOT assume `bldr setup` needs to be run - it runs automatically when bldr starts for almost any operation
- DO NOT manually copy files to `.bldr/src/` or sync source files there. `.bldr/src/` is managed by `bldr setup` and regenerated automatically. Edit source files in their original locations only.

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

This project's module is `github.com/aperturerobotics/alpha`. Local proto files reference each other using the full Go module path:

```protobuf
// In sdk/session/session.proto
import "github.com/aperturerobotics/alpha/core/session/session.proto";
import "github.com/aperturerobotics/alpha/core/sobject/sobject.proto";
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
