# CLI Compiler

The CLI compiler (`bldr/cli/compiler`) generates standalone command-line
binaries from a bldr.yaml manifest declaration. It follows the same pattern as
the plugin and dist compilers: scan Go packages for controller factories,
embed a ConfigSet, and compile a self-contained binary.

## Architecture

The CLI compiler produces a binary that boots a DevtoolBus on startup, registers
controller factories, applies an embedded ConfigSet, and exposes custom CLI
commands alongside the built-in `start` command.

```
bldr.yaml manifest
    |
    v
cli/compiler (BuildManifest)
    |-- AnalyzePackages(go_pkgs) -> discover NewFactory() functions
    |-- Resolve cli_pkgs -> discover NewCliCommands() functions
    |-- Serialize config_set -> configset.bin
    |-- FormatCliEntrypoint() -> generate main.go
    |-- go build -> standalone binary
    v
output binary
```

The generated `main.go` calls `cli_entrypoint.Main()` which:

1. Parses global flags (`--state-path`, `--log-level`, `--watch`)
2. Boots a DevtoolBus with storage and world engine
3. Registers all discovered controller factories
4. Applies the embedded ConfigSet
5. Adds the built-in `start` command (blocks until interrupted)
6. Adds all custom CLI commands from `cli_pkgs`

## Configuration

Declare a CLI manifest in `bldr.yaml`:

```yaml
manifests:
  my-cli:
    builder:
      id: bldr/cli/compiler
      config:
        goPkgs:
          - ./my/controllers
        cliPkgs:
          - ./my/cli/commands
        configSet:
          my-controller:
            id: my/controller/id
            config:
              someOption: true
```

### Config Fields

| Field       | Description                                                                                                                                                     |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `goPkgs`    | Go packages to scan for `NewFactory(b bus.Bus) controller.Factory` functions. Relative paths (starting with `./`) are resolved against the project's Go module. |
| `cliPkgs`   | Go packages providing CLI commands. Each must export `NewCliCommands(getBus func() CliBus) []*cli.Command`.                                                     |
| `configSet` | A ConfigSet to embed in the binary and apply on startup.                                                                                                        |
| `projectId` | Override the project ID used for CLI state-root defaults and environment variables. Empty keeps the generic `.bldr` defaults.                                  |

## Writing CLI Commands

Create a Go package that exports `NewCliCommands`:

```go
package my_commands

import (
    cli_entrypoint "github.com/aperturerobotics/bldr/cli/entrypoint"
    "github.com/aperturerobotics/cli"
)

// NewCliCommands builds the CLI commands for my-cli.
func NewCliCommands(getBus func() cli_entrypoint.CliBus) []*cli.Command {
    return []*cli.Command{
        {
            Name:  "greet",
            Usage: "print a greeting",
            Flags: []cli.Flag{
                &cli.StringFlag{
                    Name:  "name",
                    Value: "world",
                },
            },
            Action: func(c *cli.Context) error {
                b := getBus()
                b.GetLogger().Infof("hello, %s!", c.String("name"))
                return nil
            },
        },
    }
}
```

The `getBus()` function returns a `CliBus` interface providing access to the
ControllerBus, logger, volume, and world engine. The bus is initialized on
first access and released after the command exits (via the app's `After` hook).

### CliBus Interface

```go
type CliBus interface {
    GetContext() context.Context
    GetBus() bus.Bus
    GetLogger() *logrus.Entry
    GetVolume() volume.Volume
    GetWorldEngineID() string
    GetWorldEngine() world.Engine
    GetWorldState() world.WorldState
    GetPluginHostObjectKey() string
    Release()
}
```

## Running

Use `bldr start cli <manifest-id>` to build and run a CLI manifest in
development mode. The CLI compiler builds the binary, checks it out, and
executes it as a child process. File changes trigger automatic rebuild and
restart.

```bash
# run via bldr start
bldr start cli my-cli

# pass arguments to the CLI binary after --
bldr start cli my-cli -- --help
bldr start cli my-cli -- hello --name=world

# or via package.json script
bun start:cli
bun start:cli -- --help
```

Everything after `--` is passed as `os.Args[1:]` to the CLI binary subprocess.

### Built-in Features

The generated binary includes these built-in features:

- `start` command: boots the DevtoolBus and blocks until interrupted
- `--state-path` / `-s`: configure the `.bldr` state directory (default: `.bldr`)
- `--log-level`: set log level (debug, info, warn, error)
- `--watch` / `-w`: watch filesystem for changes

## Example

See `example/cli/` for a working example. The bldr.yaml manifest:

```yaml
manifests:
  bldr-demo-cli:
    builder:
      id: bldr/cli/compiler
      config:
        goPkgs:
          - ./example
        cliPkgs:
          - ./example/cli
        configSet:
          demo-1:
            id: bldr/example/demo
            config:
              runDemo: false
```

The `example/cli/cli.go` package exports `NewCliCommands` with `hello` and
`status` commands. The `./example` package provides `NewFactory` which
registers the demo controller.

Run with:

```bash
bun start:cli
bun start:cli -- hello --name=world
```

## Packages

| Package           | Purpose                                                                              |
| ----------------- | ------------------------------------------------------------------------------------ |
| `cli/compiler/`   | Manifest compiler controller. Scans packages, generates entrypoint, compiles binary. |
| `cli/entrypoint/` | Runtime library. `CliBus` interface, `Main()` function, type aliases.                |
