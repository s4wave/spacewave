---
title: Installation and Commands
section: cli
order: 1
summary: Build the Spacewave CLI from source and reference its commands during plugin development.
---

## Audience

This page is for contributors building Spacewave from source. If you just want
to install the CLI and use it against a desktop session, follow the
[user install guide](/docs/users/cli/install) and the
[CLI quickstart](/docs/users/cli/quickstart) instead. They cover the packaged
builds and the per-OS install commands published with each release.

## Building From Source

Contributors build the Spacewave CLI (`spacewave-cli`) directly from the
repository as a standalone Go binary that provides command-line access to all
Spacewave operations:

```bash
go build -o ~/bin/spacewave-cli ./cmd/spacewave-cli
```

The CLI connects to a running Spacewave daemon over a Unix socket. Start the daemon before using other commands.

## Core Commands

**Daemon management:**

```bash
spacewave-cli serve        # Start the daemon
spacewave-cli start        # Start the daemon inline (foreground)
spacewave-cli status       # Show daemon health
```

**Space operations:**

```bash
spacewave-cli space list              # List all spaces
spacewave-cli space create <name>     # Create a new space
spacewave-cli space info <id>         # Show space state and plugins
spacewave-cli space settings          # Display space settings
spacewave-cli space object list       # List objects in a space
spacewave-cli space object create <key> --type fs  # Create an object
```

**File operations (UnixFS):**

```bash
spacewave-cli fs ls <uri>       # List directory contents
spacewave-cli fs cat <uri>      # Read a file
spacewave-cli fs write <uri>    # Write to a file (stdin or --from)
spacewave-cli fs mkdir <uri>    # Create a directory
spacewave-cli fs rm <uri>       # Remove a file or directory
```

URI format: `/u/{session-index}/so/{space-id}/-/{object-key}/-/{path}`

## Running a Local Instance

Start a local Spacewave instance for development:

```bash
spacewave-cli serve
```

The daemon listens on a Unix socket and serves all RPC operations locally. The browser UI connects to this daemon for development and testing. Multiple CLI sessions can connect to the same daemon simultaneously.

## Plugin Development Commands

**Plugin management:**

```bash
spacewave-cli plugin list              # List plugins and approval state
spacewave-cli plugin list --watch      # Stream plugin state changes
spacewave-cli plugin add <manifest-id> # Add a plugin to a space
spacewave-cli plugin remove <id>       # Remove a plugin from a space
spacewave-cli plugin approve <name>    # Approve a pending plugin
spacewave-cli plugin deny <name>       # Deny a plugin
```

Plugin names are resolved by matching against the plugin ID, description, or partial string. The `--watch` flag on `plugin list` streams updates as plugins are loaded, approved, or removed.

## Configuration

The CLI reads configuration from `bldr.yaml` in the project root. Plugin declarations, controller registrations, and build settings are all defined in this file. The `configSet` section maps controller IDs to their protobuf configurations, which the runtime deserializes at startup.

Environment-specific settings (socket paths, log levels) are controlled through command-line flags. Run `spacewave-cli --help` for the full list of available flags and subcommands.
