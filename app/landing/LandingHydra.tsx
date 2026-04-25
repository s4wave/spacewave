import {
  LuCheck,
  LuDatabase,
  LuGithub,
  LuGlobe,
  LuHardDrive,
  LuLock,
  LuServer,
} from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { LegalPageLayout } from './LegalPageLayout.js'

export const metadata = {
  title: 'Hydra - A database that lives everywhere you do.',
  description:
    'Open-source P2P data store. Block-DAG, content-addressed, encrypted, multi-backend. The storage layer behind Spacewave.',
  canonicalPath: '/landing/hydra',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES = [
  {
    icon: LuDatabase,
    title: 'Block-DAG structure',
    description:
      'Data is organized as a directed acyclic graph of content-addressed blocks. Each block is immutable and verifiable by its hash.',
  },
  {
    icon: LuLock,
    title: 'Content-addressed',
    description:
      'Every piece of data has a unique address derived from its content. Deduplication, integrity verification, and caching are automatic.',
  },
  {
    icon: LuServer,
    title: 'Encrypted at rest',
    description:
      'Blocks are encrypted before storage. The backend sees only opaque ciphertext. Swap storage providers without re-encrypting.',
  },
  {
    icon: LuHardDrive,
    title: 'Multi-backend',
    description:
      'Store blocks on local disk, S3, IndexedDB, a Raspberry Pi, or any combination. Backends are pluggable and composable.',
  },
  {
    icon: LuGlobe,
    title: 'Cross-platform',
    description:
      'Runs in browsers (WASM), on desktops (native), and on embedded devices. Same API, same data format, everywhere.',
  },
  {
    icon: LuCheck,
    title: 'Sync protocol',
    description:
      'Devices exchange only the blocks they need. Bloom filters and set reconciliation minimize bandwidth. Works over any transport.',
  },
]

const GO_EXAMPLE = `import (
    "github.com/s4wave/spacewave/db/volume"
    "github.com/s4wave/spacewave/db/block"
)

// Open a volume backed by local disk.
vol, err := volume.Open(ctx, "/data/hydra")
if err != nil {
    return err
}
defer vol.Close()

// Write a block.
data := []byte("hello, hydra")
blk := block.NewBlock(data)
if err := vol.PutBlock(ctx, blk); err != nil {
    return err
}

// Read it back by content address.
got, err := vol.GetBlock(ctx, blk.Cid())
// got.Data() == "hello, hydra"`

// LandingHydra renders the P2P Data Store OSS component page.
export function LandingHydra() {
  const landingHref = useStaticHref('/landing')

  return (
    <LegalPageLayout
      icon={<LuDatabase className="h-8 w-8" />}
      title="A database that lives everywhere you do."
      subtitle="Hydra is an open-source, content-addressed, encrypted block store that syncs across devices over peer-to-peer connections."
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
          go get github.com/s4wave/spacewave/db@latest
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
              Hydra is the storage engine behind every Spacewave Space. Files,
              notes, chat history, and plugin data all live in Hydra volumes.
            </p>
            <p>
              Spacewave Drive, Notes, and Chat are all built on top of Hydra's
              block-DAG primitives. The same library that powers Spacewave is
              available for your projects.
            </p>
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <div className="flex flex-wrap justify-center gap-3">
          <a
            href="https://github.com/s4wave/spacewave/db"
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
