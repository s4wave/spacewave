import {
  LuDownload,
  LuLayoutGrid,
  LuTerminal,
} from '@s4wave/web/images/icons.js'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import { UseCasePage } from './use-case/UseCasePage.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    display: 'grid',
    width: '100%',
    maxWidth: 'var(--container-6xl)',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background'],
  },
  rule3: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule4: {
    overflowX: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
  },
  rule5: { color: themeColors['--color-foreground'] },
  rule6: { color: themeColors['--color-foreground-alt'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

export const metadata = {
  title: 'Spacewave CLI - Your Spaces from the terminal.',
  description:
    'The spacewave command runs the full Spacewave runtime in your terminal. Create Spaces, write files and link devices from a shell or script.',
  canonicalPath: '/landing/cli',
}

// CliStep is one group of commands in the CLI walkthrough.
interface CliStep {
  title: string
  lines: string[]
}

// CLI_STEPS walks from a new account to files, devices and the web app, using
// only commands the spacewave binary provides.
const CLI_STEPS: CliStep[] = [
  {
    title: 'Start a local account',
    lines: [
      '# Create an offline account on this machine',
      'spacewave login local',
      '# Or add this machine to an existing account',
      'spacewave login pair',
    ],
  },
  {
    title: 'Work with files',
    lines: [
      'spacewave space create "My Space"',
      'spacewave space object create --type fs files',
      'echo hello | spacewave fs write files/-/greeting.txt',
      'spacewave fs cat files/-/greeting.txt',
    ],
  },
  {
    title: 'Link a computer',
    lines: [
      '# On the new computer: print a setup ticket',
      'spacewave device setup',
      '# On a linked computer: approve it',
      'spacewave device approve <ticket>',
    ],
  },
  {
    title: 'Open the app',
    lines: ['# Serve the Spacewave web app on localhost', 'spacewave web'],
  },
]

// LandingCli renders the CLI use-case page: a walkthrough of real commands.
export function LandingCli() {
  const landingHref = useStaticHref('/landing')
  const downloadHref = useStaticHref('/download/cli')

  return (
    <UseCasePage
      icon={<LuTerminal sx={stylexStyles.rule1} />}
      title="Your Spaces from the terminal."
      subtitle="The spacewave command runs the same runtime as the app. It starts its daemon on first use, so every command works from a shell, a script or a headless server."
      points={[
        {
          title: 'One binary',
          body: 'The CLI is a single spacewave binary for Linux, macOS and Windows. It needs no browser and no other services.',
        },
        {
          title: 'Same Spaces as the app',
          body: 'A Space created in the terminal is the same Space the app opens, and files written with fs write show up there.',
        },
        {
          title: 'Built for scripts',
          body: 'Commands read stdin and write stdout, and status commands accept --output json for other tools to parse.',
        },
      ]}
      keepTitle="Install the CLI"
      keepBody="Download the spacewave binary for your platform, put it on your PATH and run spacewave status to confirm it works."
      primary={{
        href: downloadHref,
        label: 'Download the CLI',
        icon: LuDownload,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <section sx={stylexStyles2.rule1}>
        {CLI_STEPS.map((step) => (
          <div key={step.title} sx={stylexStyles2.rule2}>
            <h2 sx={stylexStyles2.rule3}>{step.title}</h2>
            <pre sx={stylexStyles2.rule4}>
              {step.lines.map((line) => (
                <div
                  key={line}
                  sx={[
                    stylexStyles2.rule5,
                    !!line.startsWith('#') && stylexStyles2.rule6,
                  ]}
                >
                  {line}
                </div>
              ))}
            </pre>
          </div>
        ))}
      </section>
    </UseCasePage>
  )
}
