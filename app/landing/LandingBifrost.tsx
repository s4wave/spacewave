import {
  LuCheck,
  LuGithub,
  LuGlobe,
  LuLock,
  LuNetwork,
  LuShield,
  LuWifi,
  LuZap,
} from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { LegalPageLayout } from './LegalPageLayout.js'

export const metadata = {
  title: 'Bifrost - Encrypted networking between any two devices on earth.',
  description:
    'Open-source P2P network router. Transport-agnostic, NAT traversal, multiplexed streams, Ed25519 identity. The networking layer behind Spacewave.',
  canonicalPath: '/landing/bifrost',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

const FEATURES = [
  {
    icon: LuGlobe,
    title: 'Transport-agnostic',
    description:
      'UDP, TCP, WebSocket, WebRTC, Bluetooth. Bifrost abstracts the transport layer so your application code works over any medium.',
  },
  {
    icon: LuWifi,
    title: 'NAT traversal',
    description:
      'Automatic hole punching, relay fallback, and signaling. Devices behind home routers, corporate firewalls, and cellular NAT connect without configuration.',
  },
  {
    icon: LuZap,
    title: 'Multiplexed streams',
    description:
      'Thousands of concurrent streams over a single connection. Yamux multiplexing with flow control and backpressure.',
  },
  {
    icon: LuShield,
    title: 'Ed25519 identity',
    description:
      'Every peer has a cryptographic identity. Connections are authenticated and encrypted. No CA infrastructure required.',
  },
  {
    icon: LuNetwork,
    title: 'Pub-sub messaging',
    description:
      'Publish messages to topics and subscribe to them across the network. Built-in support for group communication patterns.',
  },
  {
    icon: LuLock,
    title: 'End-to-end encrypted',
    description:
      'All traffic is encrypted between peers. Even relay nodes cannot read the data passing through them.',
  },
]

const GO_EXAMPLE = `import (
    "github.com/s4wave/spacewave/net/peer"
    "github.com/s4wave/spacewave/net/transport/udp"
)

// Generate a peer identity.
privKey, _, err := peer.GenerateEd25519Key()
if err != nil {
    return err
}

// Create a UDP transport.
tpt, err := udp.NewTransport(ctx, &udp.Config{
    ListenAddr: ":9001",
}, privKey)
if err != nil {
    return err
}

// Open an encrypted stream to a remote peer.
stream, err := tpt.OpenStream(ctx, remotePeerID)
if err != nil {
    return err
}
defer stream.Close()

// stream implements io.ReadWriteCloser.
stream.Write([]byte("hello, bifrost"))`

// LandingBifrost renders the P2P Network Router OSS component page.
export function LandingBifrost() {
  const landingHref = useStaticHref('/landing')

  return (
    <LegalPageLayout
      icon={<LuNetwork className="h-8 w-8" />}
      title="Encrypted networking between any two devices on earth."
      subtitle="Bifrost is an open-source peer-to-peer network router. Transport-agnostic, NAT-traversing, and cryptographically authenticated."
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
          go get github.com/s4wave/spacewave/net@latest
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
              Bifrost powers every peer-to-peer connection in Spacewave. When
              your devices sync files, send messages, or share a terminal
              session, Bifrost handles the encrypted transport.
            </p>
            <p>
              Spacewave's device linking, NAT traversal, and relay fallback are
              all built on Bifrost primitives. The same library is available for
              your own networked applications.
            </p>
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <div className="flex flex-wrap justify-center gap-3">
          <a
            href="https://github.com/s4wave/spacewave/net"
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
