---
title: Building a Go Plugin
section: plugins
order: 4
summary: Build a Spacewave plugin using Go and WebAssembly.
draft: true
---

## Go Plugin Architecture

Go plugins compile to WebAssembly and run inside the Spacewave runtime alongside the core backend. They use the same controller, directive, and world state infrastructure as the core system. Go plugins are appropriate for backend-heavy workloads: custom storage logic, data processing, network services, and controllers that need direct access to the controllerbus.

## Project Setup

A Go plugin is declared in `bldr.yaml` with the `bldr/plugin/compiler/go` builder:

```yaml
my-go-plugin:
  builder:
    id: bldr/plugin/compiler/go
    rev: 1
    config:
      goPkgs:
        - ./plugin/my-go-plugin/controller
      configSet:
        my-controller:
          id: my-go-plugin/controller
          config:
            someField: value
```

The `goPkgs` array lists Go packages that export `NewFactory` or `BuildFactories` functions. The `configSet` maps controller IDs to their protobuf configurations. At build time, the compiler scans the packages and generates a `plugin.go` with a `Factories` array.

## Defining Controllers

Each controller implements the `controller.Controller` interface with a protobuf `Config`, a constructor, and an `Execute` method:

```go
// Controller manages the plugin's background work.
type Controller struct {
    bus bus.Bus
    conf *Config
}

// NewFactory creates a factory for this controller.
func NewFactory(bus bus.Bus) controller.Factory {
    return controller.NewFactory(
        ConfigID(),
        func(conf *Config, opts controller.ConstructOpts) (
            controller.Controller, error,
        ) {
            return &Controller{bus: opts.GetBus(), conf: conf}, nil
        },
    )
}

// Execute runs the controller's main loop.
func (c *Controller) Execute(ctx context.Context) error {
    // Return nil if no background work is needed.
    // Block on ctx.Done() only if the controller has active work.
    return nil
}
```

Controllers register directive resolvers to respond to system-wide requests. Use `directive.NewValueResolver` for static values and `directive.NewFuncResolver` for async or watching logic.

## Protobuf Definitions

Define the controller's configuration and any custom messages in `.proto` files:

```protobuf
syntax = "proto3";
package my.plugin;

message Config {
  string some_field = 1;
}
```

Generate code with `git add path/to/file.proto && bun run gen`. The generated `.pb.go` and `.pb.ts` files provide typed serialization without reflection.

## Compiling to WASM

The `bldr/plugin/compiler/go` builder handles WASM compilation automatically. It compiles the Go packages with `GOOS=js GOARCH=wasm`, links the controller factories, and packages the binary as a content-addressed manifest.

The same Go code runs natively on desktop and CLI targets. The WASM build uses `protobuf-go-lite` for reflection-free serialization, which keeps binary size small and avoids the `reflect` package.

## Testing and Debugging

Test Go plugins using the testbed:

```go
func TestMyController(t *testing.T) {
    ctx := context.Background()
    tb, err := testbed.Default(ctx)
    require.NoError(t, err)
    defer tb.Close()

    // Add controller factory
    tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

    // Apply configuration
    // Verify directives resolve correctly
}
```

Use real in-memory buses and directive resolution instead of mocks. The testbed exercises the actual controller lifecycle, state management, and directive handling.
