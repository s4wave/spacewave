import {
  LuCheck,
  LuGithub,
  LuGlobe,
  LuLock,
  LuNetwork,
  LuShield,
  LuWifi,
  LuZap,
} from '@s4wave/web/images/icons.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { ExternalLink } from './ExternalLink.js'
import { LegalPageLayout } from './LegalPageLayout.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export const metadata = {
  title: 'Bifrost - Encrypted networking for any device',
  description:
    'Open-source P2P network router. Transport-agnostic, NAT traversal, multiplexed streams, Ed25519 identity. The networking layer behind Spacewave.',
  canonicalPath: '/landing/bifrost',
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
      icon={<LuNetwork sx={stylexStyles.rule1} />}
      title="Encrypted networking between any two devices on earth."
      subtitle="Bifrost is an open-source peer-to-peer network router. Transport-agnostic, NAT-traversing, and cryptographically authenticated."
    >
      {/* Features grid */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <div className="grid gap-4 @lg:grid-cols-2">
          {FEATURES.map((feature) => {
            const Icon = feature.icon
            return (
              <div key={feature.title} sx={[stylexStyles3.rule1, groupMarker]}>
                <div className="mb-3 flex items-center gap-3">
                  <div className="bg-brand/8 group-hover:bg-brand/15 flex size-9 shrink-0 items-center justify-center rounded-lg transition-colors">
                    <Icon sx={stylexStyles2.rule1} />
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
        <h2 className="text-foreground mb-6 text-center text-xl font-semibold">
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
          <h2 className="text-foreground mb-4 text-center text-xl font-semibold">
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
          <ExternalLink
            href="https://github.com/s4wave/spacewave/net"
            className={cn(
              'border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15',
              'flex items-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium no-underline transition-all duration-300 select-none hover:-translate-y-0.5',
            )}
          >
            <LuGithub sx={stylexStyles.rule2} />
            <span>View on GitHub</span>
          </ExternalLink>
          <a
            href={landingHref}
            className="border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8 flex items-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium no-underline transition duration-300 select-none hover:-translate-y-0.5"
          >
            <LuCheck sx={stylexStyles.rule2} />
            <span>See all features</span>
          </a>
        </div>
      </section>
    </LegalPageLayout>
  )
}
