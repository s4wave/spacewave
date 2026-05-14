# spacewave

Command-line interface for Spacewave. Manages the local daemon, sessions,
spaces, and all data operations (files, git repos, canvas).

## Quick Start

```bash
# Sign up or log in
spacewave login --username alice

# Or sign in via browser handoff
spacewave login browser
spacewave login --browser

# Or create a local offline account
spacewave login local

# Check status
spacewave status

# Start the daemon explicitly
spacewave serve

# Create a space and start working
spacewave space create my-project
spacewave space object create docs --type fs
spacewave fs write /u/1/so/SPACE_ID/-/docs/-/hello.txt --from hello.txt
```

## Global Flags

| Flag | Env | Description |
|---|---|---|
| `--state-path, -s` | `BLDR_STATE_PATH` | State directory (default: `.bldr`) |
| `--output, -o` | `SPACEWAVE_CLI_OUTPUT` | Output format: `text` (default), `json`, `yaml` |
| `--color` | `SPACEWAVE_CLI_COLOR` | Color mode: `auto`, `always`, `never` |
| `--log-level` | `BLDR_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` |
| `--log-file` | `BLDR_LOG_FILE` | File logging spec |

Many commands also accept `--session-index` (default: 1) to select which
session to operate on. Client commands connect to an existing daemon by
default. Use `--autostart` or `SPACEWAVE_CLI_AUTOSTART=true` only when you want
the CLI to start a background daemon if none is reachable.

## Command Reference

### Getting Started

```
spacewave login [--username USER]  Sign up or log in (cloud)
spacewave login browser           Sign in via browser handoff
spacewave login local             Create a local offline account
spacewave logout                  Revoke the current cloud session
spacewave whoami                  Show current session identity
spacewave status                  Daemon health, session, auth state, spaces
```

`login` handles both signup and login. If the username exists, it logs in;
otherwise it creates a new account. `--password` or `SPACEWAVE_PASSWORD`
provides the password non-interactively. Use `login browser` or `login
--browser` to route auth through the browser handoff flow.

### Daemon

```
spacewave serve              Start daemon and listen for CLI connections
spacewave serve --takeover   Ask an existing runtime to yield its socket
spacewave start              Start daemon in foreground without socket
```

`serve` listens on a Unix socket at `STATE_PATH/spacewave.sock`. Runtime-dependent
commands connect to that socket as clients and do not start or take over a
daemon unless `--autostart` is set. If another runtime already owns the socket,
plain `serve` fails instead of taking it over; use `serve --takeover` only when
you intentionally want that runtime to yield. `start` runs the bus inline
without a socket (for development and testing).

In desktop builds, the Electron tray/menu-bar runtime also owns a daemon
socket. Closing every renderer window leaves that runtime alive when
tray-backed desktop presence is enabled, so installed CLI commands such as
`spacewave status --socket-path <desktop-socket>` continue to connect until the
user explicitly quits from the tray/menu or app.

### Auth

Authentication, locking, and credential management.

```
spacewave auth lock                Lock the session now
spacewave auth lock pin            Set PIN lock mode
spacewave auth lock auto           Set auto-unlock mode
spacewave auth lock status         Show lock mode and locked state
spacewave auth unlock              Unlock a PIN-locked session
spacewave auth passwd              Change the account password
spacewave auth method list         List registered entity keypairs
spacewave auth method add password Add a password-derived keypair
spacewave auth method add pem      Add a PEM file as an auth method
spacewave auth method add backup   Generate + register a backup key, save PEM
spacewave auth method remove <id>  Remove an auth method by peer ID
spacewave auth threshold           Show current auth threshold
spacewave auth threshold set <N>   Set multi-sig auth threshold
```

### Spaces

A space is a collaborative container holding world objects (files, repos,
canvases) synced across devices.

```
spacewave space list                 List spaces (supports --watch)
spacewave space create <name>        Create a new space
spacewave space info <space-id>      Show space state, objects, plugins
spacewave space resolve <name>       Resolve a space name to its ID
spacewave space settings             Show space settings (index path, plugins)
spacewave space import-git <url>     Import a git repo into a space
spacewave space deploy               Deploy a manifest from a .bldr devtool DB
```

#### World Objects

Objects are typed data containers within a space. Types: `fs` (files), `git`
(repository), `canvas`, `canvas-demo`.

```
spacewave space object list                    List objects (supports --watch)
spacewave space object info <key>              Show object state and root ref
spacewave space object create <key> --type fs  Create an object
spacewave space object delete <key>            Delete an object
```

Use `--space-id` or `--space` to target a space. Auto-detected if only one
space exists.

### Files (UnixFS)

File operations on `fs`-type objects. All commands take a URI that encodes
the session, space, object, and path:

```
/u/{session-index}/so/{space-id}/-/{object-key}/-/{path}
```

```
spacewave fs ls <uri>                    List directory contents
spacewave fs cat <uri>                   Read file contents to stdout
spacewave fs write <uri>                 Write to a file (stdin or --from)
spacewave fs mkdir <uri>                 Create directory (and parents)
spacewave fs rm <uri>                    Remove file or directory
spacewave fs mv <source-uri> <dest-uri>  Move/rename a file or directory
spacewave fs stat <uri>                  Show file info (name, type, size, mode)
```

`cat` supports `--offset` and `--limit` for partial reads. `write` accepts
`--from <path>` to read from a local file instead of stdin.

### Git

Git operations on `git`-type objects. Commands accept `--uri` / `--git` / `--repo`
to identify the repository, or auto-detect from space context.

```
spacewave git show                Show repo overview (HEAD, last commit)
spacewave git refs                List branches and tags
spacewave git log                 Show commit history (--ref, --since, --limit, --offset)
spacewave git diff <refA> [refB]  Show diff stats between refs
spacewave git commit <ref>        Show commit details with diff stats
spacewave git tree [path]         Browse files in a ref's tree (--ref)
spacewave git clone               Clone a remote repository into the world
spacewave git fetch               Fetch updates from remote
```

#### Git Worktrees

```
spacewave git worktree create     Create a worktree (--branch or --commit)
spacewave git worktree checkout   Checkout a revision in an existing worktree
```

### Canvas

Canvas operations on `canvas`-type objects. Commands accept `--uri` / `--canvas`
to identify the canvas.

```
spacewave canvas show             Show canvas summary (node/edge counts, bounds)
spacewave canvas watch            Stream canvas state changes
spacewave canvas export           Export full canvas state as JSON or YAML
spacewave canvas apply            Apply a world op from stdin or --from file
```

#### Canvas Nodes

```
spacewave canvas node list                  List nodes (ID, type, position, size)
spacewave canvas node add text              Add a text node
spacewave canvas node add object            Add a world-object node
spacewave canvas node add shape             Add a shape node
spacewave canvas node add drawing           Add a drawing node
spacewave canvas node rm <id> [id...]       Remove nodes
spacewave canvas node set --node <id>       Update node properties (-x, -y, --width, --height, -z, --text, --pinned)
spacewave canvas node navigate --node <id>  Set node viewer path
```

#### Canvas Edges

```
spacewave canvas edge list                          List edges
spacewave canvas edge add --source <id> --target <id>  Add an edge (--label, --style bezier|straight)
spacewave canvas edge rm <id> [id...]               Remove edges
```

### VMs

VM operations for spaces. The first runtime is V86, with runtime-specific VM
creation and image import commands under `vm create v86` and `vm image v86`.

```
spacewave vm list --space <space-id>       List VMs
spacewave vm info <vm-key> --space <space-id>  Show VM details
spacewave vm create v86 <name> --image <v86-image-key> --space <space-id>  Create a V86 VM
spacewave vm start <vm-key> --space <space-id> [--wait]  Start a VM
spacewave vm stop <vm-key> --space <space-id>  Stop a VM
spacewave vm watch <vm-key> --space <space-id>  Stream VM state changes
```

`vm create v86` accepts `--memory-mb`, `--vga-memory-mb`, and repeated
`--mount /guest/path=objectKey[:rw|:ro]` flags.

#### V86 Images

```
spacewave vm image v86 list --space <space-id>       List V86 images
spacewave vm image v86 info <image-key> --space <space-id>  Show V86 image details
spacewave vm image v86 copy-from-cdn <cdn-image-key> --space <space-id> [--as <image-key>]  Copy a CDN V86 image into a space
spacewave vm image v86 import tar --space <space-id> --name <name> --wasm <v86.wasm> --seabios <seabios.bin> --vgabios <vgabios.bin> --kernel <bzImage> --rootfs-tar <rootfs.tar> [--as <image-key>] [--tag <tag>]  Create a V86 image from local files
```

### Plugins

Plugin management for spaces. Plugins extend space functionality (e.g., chat,
viewers).

```
spacewave plugin list                   List plugins and load state (--watch)
spacewave plugin add <manifest-id>      Add a plugin to space settings
spacewave plugin remove <manifest-id>   Remove a plugin from space settings
```

### Plumbing

Lower-level commands for scripting and automation. These expose the internal
provider/account/session model directly.

```
spacewave account list                         List accounts grouped by provider
spacewave account info                         Show account details (entity ID, threshold, keypairs)
spacewave account create local                 Create a local offline account + session
spacewave account create spacewave --username  Create a Spacewave cloud account + session
spacewave session list                         List all sessions
spacewave session info                         Show session details with spaces list
spacewave provider list                        List registered providers
spacewave provider info                        Show provider details and features
```

## Output Formats

All commands support `--output` / `-o`:

- `text` (default): human-readable aligned tables and key-value pairs
- `json`: machine-readable JSON
- `yaml`: YAML (converted from JSON)

When stdout is piped, text output is plain (no color, no truncation).

## Environment Variables

Commands accept configuration via environment variables as an alternative to
flags. Key variables:

| Variable                  | Description                               |
|---------------------------|-------------------------------------------|
| `SPACEWAVE_STATE_PATH`    | Daemon state directory                    |
| `SPACEWAVE_SESSION_INDEX` | Default session index                     |
| `SPACEWAVE_SPACE`         | Default space ID                          |
| `SPACEWAVE_PASSWORD`      | Account password (avoid shell history)    |
| `SPACEWAVE_USERNAME`      | Account username                          |
| `SPACEWAVE_OUTPUT`        | Default output format                     |
| `SPACEWAVE_WATCH`         | Enable watch mode                         |
| `NO_COLOR`                | Disable color output (community standard) |
