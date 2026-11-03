import {
  LuCheck,
  LuDatabase,
  LuGithub,
  LuGlobe,
  LuHardDrive,
  LuLock,
  LuServer,
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
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles5 = stylex.create({
  rule1: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBottom: `calc(${themeLayout['--spacing']} * 16)`,
  },
  rule2: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    textWrap: 'balance',
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule8: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-dark'],
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule10: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    overflowX: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
  },
  rule12: { color: themeColors['--color-foreground-alt'] },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule14: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule15: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule16: {
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule17: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    textDecorationLine: 'none',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
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

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    textDecorationLine: 'none',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
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
  title: 'Hydra - A database that lives everywhere you do.',
  description:
    'Open-source P2P data store. Block-DAG, content-addressed, encrypted, multi-backend. The storage layer behind Spacewave apps.',
  canonicalPath: '/landing/hydra',
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
      icon={<LuDatabase sx={stylexStyles.rule1} />}
      title="A database that lives everywhere you do."
      subtitle="Hydra is an open-source, content-addressed, encrypted block store that syncs across devices over peer-to-peer connections."
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
          go get github.com/s4wave/spacewave/db@latest
        </div>
      </section>

      {/* Used in Spacewave callout */}
      <section sx={stylexStyles5.rule1}>
        <div sx={stylexStyles5.rule14}>
          <h2 sx={stylexStyles5.rule15}>Used in Spacewave</h2>
          <div sx={stylexStyles6.rule1}>
            <p sx={nativeChildSpacing.s3}>
              Hydra is the storage engine behind every Spacewave Space. Files,
              notes, chat history, and plugin data all live in Hydra volumes.
            </p>
            <p sx={nativeChildSpacing.s3}>
              Spacewave Drive, Notes, and Chat are all built on top of Hydra's
              block-DAG primitives. The same library that powers Spacewave is
              available for your projects.
            </p>
          </div>
        </div>
      </section>

      {/* CTA */}
      <section sx={stylexStyles5.rule1}>
        <div sx={stylexStyles5.rule16}>
          <ExternalLink
            href="https://github.com/s4wave/spacewave/db"
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
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
