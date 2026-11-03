import {
  LuCheck,
  LuCpu,
  LuGithub,
  LuLayers,
  LuRefreshCw,
  LuSettings,
  LuShield,
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
  title: 'ControllerBus - The kernel for your distributed system.',
  description:
    'Open-source controller coordination framework. Hot-reload, directive-based, deterministic lifecycle, protobuf config. The coordination layer behind Spacewave.',
  canonicalPath: '/landing/controllerbus',
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
      icon={<LuCpu sx={stylexStyles.rule1} />}
      title="The kernel for your distributed system."
      subtitle="ControllerBus is an open-source framework for coordinating controllers with hot-reload, directive resolution, and deterministic lifecycle management."
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
          go get github.com/aperturerobotics/controllerbus@latest
        </div>
      </section>

      {/* Used in Spacewave callout */}
      <section sx={stylexStyles5.rule1}>
        <div sx={stylexStyles5.rule14}>
          <h2 sx={stylexStyles5.rule15}>Used in Spacewave</h2>
          <div sx={stylexStyles6.rule1}>
            <p sx={nativeChildSpacing.s3}>
              ControllerBus is the coordination kernel at the heart of
              Spacewave. Every controller in the system (networking, storage,
              plugins, UI) is managed by the bus.
            </p>
            <p sx={nativeChildSpacing.s3}>
              When you install a plugin, ControllerBus hot-loads it. When a
              device disconnects, ControllerBus tears down the associated
              controllers cleanly. The entire Spacewave lifecycle is bus-driven.
            </p>
          </div>
        </div>
      </section>

      {/* CTA */}
      <section sx={stylexStyles5.rule1}>
        <div sx={stylexStyles5.rule16}>
          <ExternalLink
            href="https://github.com/aperturerobotics/controllerbus"
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
