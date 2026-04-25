import {
  LuCheck,
  LuCpu,
  LuGithub,
  LuLayers,
  LuRefreshCw,
  LuSettings,
  LuShield,
  LuZap,
} from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { LegalPageLayout } from './LegalPageLayout.js'

export const metadata = {
  title: 'ControllerBus - The kernel for your distributed system.',
  description:
    'Open-source controller coordination framework. Hot-reload, directive-based, deterministic lifecycle, protobuf config. The coordination layer behind Spacewave.',
  canonicalPath: '/landing/controllerbus',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES = [
  {
    icon: LuRefreshCw,
    title: 'Hot-reload',
    description:
      'Swap controller implementations at runtime without restarting the process. Load new plugins, update configurations, and patch behavior live.',
  },
  {
    icon: LuZap,
    title: 'Directive-based',
    description:
      'Controllers communicate through directives: declarative requests for capabilities. The bus resolves directives to running controllers automatically.',
  },
  {
    icon: LuShield,
    title: 'Deterministic lifecycle',
    description:
      'Controllers have a well-defined lifecycle: construct, execute, release. Error handling, retry, and backoff are built into the framework.',
  },
  {
    icon: LuSettings,
    title: 'Protobuf config',
    description:
      'Controller configuration is defined in protobuf. Type-safe, versionable, and serializable. Configuration changes trigger controller restarts automatically.',
  },
  {
    icon: LuLayers,
    title: 'Composable',
    description:
      'Controllers can depend on other controllers via directives. Build complex systems from small, focused components that compose cleanly.',
  },
  {
    icon: LuCpu,
    title: 'Cross-platform',
    description:
      'Runs on any platform Go supports: Linux, macOS, Windows, WASM. The same controller code works in browsers and on servers.',
  },
]

const GO_EXAMPLE = `import (
    "github.com/aperturerobotics/controllerbus/bus"
    "github.com/aperturerobotics/controllerbus/controller"
)

// Define a controller with protobuf config.
type MyController struct {
    config *Config
}

func (c *MyController) Execute(
    ctx context.Context,
) error {
    // Controller is running.
    // Return nil to stay alive until context cancels.
    // Return error to trigger restart with backoff.
    <-ctx.Done()
    return ctx.Err()
}

// Register and run on a bus.
b, err := bus.NewBus(ctx)
if err != nil {
    return err
}
b.AddFactory(NewFactory())

// Apply configuration (triggers controller start).
err = b.ApplyConfig(ctx, myConfig)`

// LandingControllerbus renders the Controller Coordination OSS component page.
export function LandingControllerbus() {
  const landingHref = useStaticHref('/landing')

  return (
    <LegalPageLayout
      icon={<LuCpu className="h-8 w-8" />}
      title="The kernel for your distributed system."
      subtitle="ControllerBus is an open-source framework for coordinating controllers with hot-reload, directive resolution, and deterministic lifecycle management."
    >
      {/* Features grid */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <div className="grid gap-4 @lg:grid-cols-2">
          {FEATURES.map((feature) => {
            const Icon = feature.icon
            return (
              <div
                key={feature.title}
                className="border-foreground/6 bg-background-card/30 group rounded-lg border p-6 backdrop-blur-sm transition-all duration-300 hover:-translate-y-0.5"
              >
                <div className="mb-3 flex items-center gap-3">
                  <div className="bg-brand/8 group-hover:bg-brand/15 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors">
                    <Icon className="text-brand h-4 w-4" />
                  </div>
                  <h3 className="text-foreground text-sm font-semibold">
                    {feature.title}
                  </h3>
                </div>
                <p className="text-foreground-alt text-sm leading-relaxed text-balance">
                  {feature.description}
                </p>
              </div>
            )
          })}
        </div>
      </section>

      {/* Code example */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <h2 className="text-foreground mb-6 text-center text-xl font-bold">
          Get started in Go
        </h2>
        <div className="border-foreground/10 bg-background-dark overflow-hidden rounded-lg border">
          <div className="border-foreground/8 flex items-center gap-2 border-b px-4 py-2.5">
            <span className="text-foreground-alt font-mono text-xs">
              main.go
            </span>
          </div>
          <pre className="overflow-x-auto p-4 font-mono text-sm leading-relaxed">
            <code className="text-foreground-alt">{GO_EXAMPLE}</code>
          </pre>
        </div>
        <div className="text-foreground-alt mt-4 text-center font-mono text-xs">
          go get github.com/aperturerobotics/controllerbus@latest
        </div>
      </section>

      {/* Used in Spacewave callout */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <div className="border-brand/20 bg-brand/5 rounded-lg border p-8">
          <h2 className="text-foreground mb-4 text-center text-xl font-bold">
            Used in Spacewave
          </h2>
          <div className="text-foreground-alt space-y-3 text-center text-sm leading-relaxed">
            <p>
              ControllerBus is the coordination kernel at the heart of
              Spacewave. Every controller in the system (networking, storage,
              plugins, UI) is managed by the bus.
            </p>
            <p>
              When you install a plugin, ControllerBus hot-loads it. When a
              device disconnects, ControllerBus tears down the associated
              controllers cleanly. The entire Spacewave lifecycle is bus-driven.
            </p>
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <div className="flex flex-wrap justify-center gap-3">
          <a
            href="https://github.com/aperturerobotics/controllerbus"
            className={cn(
              'border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15',
              'flex items-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium no-underline transition-all duration-300 select-none hover:-translate-y-0.5',
            )}
          >
            <LuGithub className="h-4 w-4" />
            <span>View on GitHub</span>
          </a>
          <a
            href={landingHref}
            className="border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8 flex items-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium no-underline transition-all duration-300 select-none hover:-translate-y-0.5"
          >
            <LuCheck className="h-4 w-4" />
            <span>See all features</span>
          </a>
        </div>
      </section>
    </LegalPageLayout>
  )
}
