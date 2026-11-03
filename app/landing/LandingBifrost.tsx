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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles6 = stylex.create({
  rule1: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    textAlign: 'center',
  },
})

const stylexStyles5 = stylex.create({
  rule1: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: `calc(${themeLayout['--spacing']} * 16)`,
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    textWrap: 'balance',
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    textAlign: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-dark'],
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule10: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule11: {
    '--tw-leading': 'var(--leading-relaxed)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    overflowX: 'auto',
  },
  rule12: { color: themeColors['--color-foreground-alt'] },
  rule13: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule14: {
    padding: `calc(${themeLayout['--spacing']} * 8)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule15: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    textAlign: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule16: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
  },
  rule17: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
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
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
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
      <section sx={stylexStyles5.rule1}>
        <div sx={stylexStyles5.rule2}>
          {FEATURES.map((feature) => {
            const Icon = feature.icon
            return (
              <div key={feature.title} sx={[stylexStyles3.rule1, groupMarker]}>
                <div sx={stylexStyles5.rule3}>
                  <div sx={stylexStyles5.rule4}>
                    <Icon sx={stylexStyles2.rule1} />
                  </div>
                  <h3 sx={stylexStyles5.rule5}>{feature.title}</h3>
                </div>
                <p sx={stylexStyles5.rule6}>{feature.description}</p>
              </div>
            )
          })}
        </div>
      </section>

      {/* Code example */}
      <section sx={stylexStyles5.rule1}>
        <h2 sx={stylexStyles5.rule7}>Get started in Go</h2>
        <div sx={stylexStyles5.rule8}>
          <div sx={stylexStyles5.rule9}>
            <span sx={stylexStyles5.rule10}>main.go</span>
          </div>
          <pre sx={stylexStyles5.rule11}>
            <code sx={stylexStyles5.rule12}>{GO_EXAMPLE}</code>
          </pre>
        </div>
        <div sx={stylexStyles5.rule13}>
          go get github.com/s4wave/spacewave/net@latest
        </div>
      </section>

      {/* Used in Spacewave callout */}
      <section sx={stylexStyles5.rule1}>
        <div sx={stylexStyles5.rule14}>
          <h2 sx={stylexStyles5.rule15}>Used in Spacewave</h2>
          <div sx={stylexStyles6.rule1}>
            <p sx={nativeChildSpacing.s3}>
              Bifrost powers every peer-to-peer connection in Spacewave. When
              your devices sync files, send messages, or share a terminal
              session, Bifrost handles the encrypted transport.
            </p>
            <p sx={nativeChildSpacing.s3}>
              Spacewave's device linking, NAT traversal, and relay fallback are
              all built on Bifrost primitives. The same library is available for
              your own networked applications.
            </p>
          </div>
        </div>
      </section>

      {/* CTA */}
      <section sx={stylexStyles5.rule1}>
        <div sx={stylexStyles5.rule16}>
          <ExternalLink
            href="https://github.com/s4wave/spacewave/net"
            sx={[stylexStyles4.rule1]}
          >
            <LuGithub sx={stylexStyles.rule2} />
            <span>View on GitHub</span>
          </ExternalLink>
          <a href={landingHref} sx={stylexStyles5.rule17}>
            <LuCheck sx={stylexStyles.rule2} />
            <span>See all features</span>
          </a>
        </div>
      </section>
    </LegalPageLayout>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
