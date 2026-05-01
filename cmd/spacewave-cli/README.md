# spacewave-cli

Command-line interface for Spacewave. Manages the local daemon, sessions,
spaces, and all data operations (files, git repos, canvas).

## Quick Start

```bash
# Sign up or log in
spacewave-cli login --username alice

# Or sign in via browser handoff
spacewave-cli login browser
spacewave-cli login --browser

# Or create a local offline account
spacewave-cli login local

# Check status
spacewave-cli status

# Start the daemon explicitly (optional)
spacewave-cli serve

# Create a space and start working
spacewave-cli space create my-project
spacewave-cli space object create docs --type fs
spacewave-cli fs write /u/1/so/SPACE_ID/-/docs/-/hello.txt --from hello.txt
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
session to operate on.

## Command Reference

### Getting Started

```
spacewave-cli login [--username USER]  Sign up or log in (cloud)
spacewave-cli login browser           Sign in via browser handoff
spacewave-cli login local             Create a local offline account
spacewave-cli logout                  Revoke the current cloud session
spacewave-cli whoami                  Show current session identity
spacewave-cli status                  Daemon health, session, auth state, spaces
```

`login` handles both signup and login. If the username exists, it logs in;
otherwise it creates a new account. `--password` or `SPACEWAVE_PASSWORD`
provides the password non-interactively. Use `login browser` or `login
--browser` to route auth through the browser handoff flow.

### Daemon

```
spacewave-cli serve              Start daemon and listen for CLI connections
spacewave-cli start              Start daemon in foreground without socket
```

`serve` listens on a Unix socket at `STATE_PATH/spacewave.sock`. Runtime-dependent
commands auto-start this background daemon when the socket is absent, so
explicit `serve` is mainly for foreground inspection or keeping the daemon
running yourself. `start` runs the bus inline without a socket (for development
and testing).

### Auth

Authentication, locking, and credential management.

```
spacewave-cli auth lock                Lock the session now
spacewave-cli auth lock pin            Set PIN lock mode
spacewave-cli auth lock auto           Set auto-unlock mode
spacewave-cli auth lock status         Show lock mode and locked state
spacewave-cli auth unlock              Unlock a PIN-locked session
spacewave-cli auth passwd              Change the account password
spacewave-cli auth method list         List registered entity keypairs
spacewave-cli auth method add password Add a password-derived keypair
spacewave-cli auth method add pem      Add a PEM file as an auth method
spacewave-cli auth method add backup   Generate + register a backup key, save PEM
spacewave-cli auth method remove <id>  Remove an auth method by peer ID
spacewave-cli auth threshold           Show current auth threshold
spacewave-cli auth threshold set <N>   Set multi-sig auth threshold
```

### Spaces

A space is a collaborative container holding world objects (files, repos,
canvases) synced across devices.

```
spacewave-cli space list                 List spaces (supports --watch)
spacewave-cli space create <name>        Create a new space
spacewave-cli space info <space-id>      Show space state, objects, plugins
spacewave-cli space resolve <name>       Resolve a space name to its ID
spacewave-cli space settings             Show space settings (index path, plugins)
spacewave-cli space import-git <url>     Import a git repo into a space
spacewave-cli space deploy               Deploy a manifest from a .bldr devtool DB
```

#### World Objects

Objects are typed data containers within a space. Types: `fs` (files), `git`
(repository), `canvas`, `canvas-demo`.

```
spacewave-cli space object list                    List objects (supports --watch)
spacewave-cli space object info <key>              Show object state and root ref
spacewave-cli space object create <key> --type fs  Create an object
spacewave-cli space object delete <key>            Delete an object
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
spacewave-cli fs ls <uri>                    List directory contents
spacewave-cli fs cat <uri>                   Read file contents to stdout
spacewave-cli fs write <uri>                 Write to a file (stdin or --from)
spacewave-cli fs mkdir <uri>                 Create directory (and parents)
spacewave-cli fs rm <uri>                    Remove file or directory
spacewave-cli fs mv <source-uri> <dest-uri>  Move/rename a file or directory
spacewave-cli fs stat <uri>                  Show file info (name, type, size, mode)
```

`cat` supports `--offset` and `--limit` for partial reads. `write` accepts
`--from <path>` to read from a local file instead of stdin.

### Git

Git operations on `git`-type objects. Commands accept `--uri` / `--git` / `--repo`
to identify the repository, or auto-detect from space context.

```
spacewave-cli git show                Show repo overview (HEAD, last commit)
spacewave-cli git refs                List branches and tags
spacewave-cli git log                 Show commit history (--ref, --since, --limit, --offset)
spacewave-cli git diff <refA> [refB]  Show diff stats between refs
spacewave-cli git commit <ref>        Show commit details with diff stats
spacewave-cli git tree [path]         Browse files in a ref's tree (--ref)
spacewave-cli git clone               Clone a remote repository into the world
spacewave-cli git fetch               Fetch updates from remote
```

#### Git Worktrees

```
spacewave-cli git worktree create     Create a worktree (--branch or --commit)
spacewave-cli git worktree checkout   Checkout a revision in an existing worktree
```

### Canvas

Canvas operations on `canvas`-type objects. Commands accept `--uri` / `--canvas`
to identify the canvas.

```
spacewave-cli canvas show             Show canvas summary (node/edge counts, bounds)
spacewave-cli canvas watch            Stream canvas state changes
spacewave-cli canvas export           Export full canvas state as JSON or YAML
spacewave-cli canvas apply            Apply a world op from stdin or --from file
```

#### Canvas Nodes

```
spacewave-cli canvas node list                  List nodes (ID, type, position, size)
spacewave-cli canvas node add text              Add a text node
spacewave-cli canvas node add object            Add a world-object node
spacewave-cli canvas node add shape             Add a shape node
spacewave-cli canvas node add drawing           Add a drawing node
spacewave-cli canvas node rm <id> [id...]       Remove nodes
spacewave-cli canvas node set --node <id>       Update node properties (-x, -y, --width, --height, -z, --text, --pinned)
spacewave-cli canvas node navigate --node <id>  Set node viewer path
```

#### Canvas Edges

```
spacewave-cli canvas edge list                          List edges
spacewave-cli canvas edge add --source <id> --target <id>  Add an edge (--label, --style bezier|straight)
spacewave-cli canvas edge rm <id> [id...]               Remove edges
```

### VMs

VM operations for spaces. The first runtime is V86, with runtime-specific VM
creation and image import commands under `vm create v86` and `vm image v86`.

```
spacewave-cli vm list --space <space-id>       List VMs
spacewave-cli vm info <vm-key> --space <space-id>  Show VM details
spacewave-cli vm create v86 <name> --image <v86-image-key> --space <space-id>  Create a V86 VM
spacewave-cli vm start <vm-key> --space <space-id> [--wait]  Start a VM
spacewave-cli vm stop <vm-key> --space <space-id>  Stop a VM
spacewave-cli vm watch <vm-key> --space <space-id>  Stream VM state changes
```

`vm create v86` accepts `--memory-mb`, `--vga-memory-mb`, and repeated
`--mount /guest/path=objectKey[:rw|:ro]` flags.

#### V86 Images

```
spacewave-cli vm image v86 list --space <space-id>       List V86 images
spacewave-cli vm image v86 info <image-key> --space <space-id>  Show V86 image details
spacewave-cli vm image v86 copy-from-cdn <cdn-image-key> --space <space-id> [--as <image-key>]  Copy a CDN V86 image into a space
spacewave-cli vm image v86 import tar --space <space-id> --name <name> --wasm <v86.wasm> --seabios <seabios.bin> --vgabios <vgabios.bin> --kernel <bzImage> --rootfs-tar <rootfs.tar> [--as <image-key>] [--tag <tag>]  Create a V86 image from local files
```

### Plugins

Plugin management for spaces. Plugins extend space functionality (e.g., chat,
viewers).

```
spacewave-cli plugin list                   List plugins and approval state (--watch)
spacewave-cli plugin approve <name-or-id>   Approve a plugin
spacewave-cli plugin deny <name-or-id>      Deny a plugin
spacewave-cli plugin add <manifest-id>      Add a plugin to space settings
spacewave-cli plugin remove <manifest-id>   Remove a plugin from space settings
```

### Plumbing

Lower-level commands for scripting and automation. These expose the internal
provider/account/session model directly.

```
spacewave-cli account list                         List accounts grouped by provider
spacewave-cli account info                         Show account details (entity ID, threshold, keypairs)
spacewave-cli account create local                 Create a local offline account + session
spacewave-cli account create spacewave --username  Create a Spacewave cloud account + session
spacewave-cli session list                         List all sessions
spacewave-cli session info                         Show session details with spaces list
spacewave-cli provider list                        List registered providers
spacewave-cli provider info                        Show provider details and features
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
