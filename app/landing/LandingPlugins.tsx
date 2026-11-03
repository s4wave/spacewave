import type { ComponentType } from 'react'
import {
  LuDatabase,
  LuLayoutGrid,
  LuPuzzle,
  LuTable,
} from '@s4wave/web/images/icons.js'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import type { LiveDemoAppProps } from './use-case/LiveDemoApp.js'
import { UseCaseDemo } from './use-case/UseCaseDemo.js'
import { UseCasePage } from './use-case/UseCasePage.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 48)`,
    flexShrink: 0,
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBottom: themeLayout['--spacing'],
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-foreground'],
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-foreground'],
  },
  rule7: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule9: {
    width: '100%',
    textAlign: 'left',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: { color: themeColors['--color-foreground-alt'] },
  rule11: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule12: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule13: { color: themeColors['--color-foreground'] },
  rule14: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule15: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
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
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

// PLUGINS_DEMO_QUERY is the query the demo suggests running.
const PLUGINS_DEMO_QUERY = 'SELECT name, role FROM quickstart.people'

export const metadata = {
  title: 'Spacewave Plugins - New kinds of objects, in your Space.',
  description:
    'Try the Spacewave SQL Database plugin in your browser. Plugins in Go or TypeScript add object types and viewers that run on your own devices.',
  canonicalPath: '/landing/plugins',
}

// LandingPlugins renders the Plugins use-case page. liveApp mounts the demo SQL
// Database, which a plugin provides.
export function LandingPlugins({
  liveApp,
}: {
  liveApp?: ComponentType<LiveDemoAppProps>
}) {
  const landingHref = useStaticHref('/landing')

  return (
    <UseCasePage
      icon={<LuPuzzle sx={stylexStyles.rule1} />}
      title="New kinds of objects, in your Space."
      subtitle="Plugins teach Spacewave new object types. The SQL Database below is a plugin: it loads when the Space needs it and runs on your device, not on a server."
      points={[
        {
          title: 'Go or TypeScript',
          body: 'The SQL Database plugin is written in Go and the Notebook in TypeScript. Both use the same SDK to register object types, viewers and quickstarts.',
        },
        {
          title: 'Loaded on demand',
          body: 'A Space loads the plugins its objects need. Each plugin runs in its own worker beside the app.',
        },
        {
          title: 'Data stays in the Space',
          body: 'Plugin objects are stored in the Space like files and notes, so they sync and back up the same way.',
        },
      ]}
      keepTitle="Keep your database"
      keepBody="The demo lives in memory and is gone when you leave. The SQL Database Quickstart creates the same Space in your browser's storage, where it stays until you delete it."
      primary={{
        href: '#/quickstart/sql',
        label: 'Create a SQL Database',
        icon: LuDatabase,
      }}
      secondary={{
        href: landingHref,
        label: 'See all features',
        icon: LuLayoutGrid,
      }}
    >
      <UseCaseDemo
        demo="plugins"
        liveApp={liveApp}
        label="SQL Database"
        poster={<PluginsPoster />}
        suggestions={[
          'Expand the quickstart schema to list its tables.',
          `Open Query Editor and run ${PLUGINS_DEMO_QUERY}.`,
          'Change the query, run it again, then press Reset to start over.',
        ]}
      />
    </UseCasePage>
  )
}

// PluginsPoster previews the SQL Database Quickstart: its schemas and the
// result of the suggested query.
function PluginsPoster() {
  const rows = [
    { name: 'ada', role: 'analyst' },
    { name: 'grace', role: 'engineer' },
  ]

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>My SQL Database</span>
        <span sx={stylexStyles2.rule4}>
          <LuDatabase sx={stylexStyles.rule2} />
          information_schema
        </span>
        <span sx={stylexStyles2.rule4}>
          <LuDatabase sx={stylexStyles.rule3} />
          quickstart
        </span>
        <span sx={stylexStyles2.rule5}>
          <LuTable sx={stylexStyles.rule2} />
          people
        </span>
        <span sx={stylexStyles2.rule6}>
          <LuTable sx={stylexStyles.rule2} />
          projects
        </span>
      </div>
      <div sx={stylexStyles2.rule7}>
        <code sx={stylexStyles2.rule8}>{PLUGINS_DEMO_QUERY}</code>
        <table sx={stylexStyles2.rule9}>
          <thead sx={stylexStyles2.rule10}>
            <tr sx={stylexStyles2.rule11}>
              <th sx={stylexStyles2.rule12}>name</th>
              <th sx={stylexStyles2.rule12}>role</th>
            </tr>
          </thead>
          <tbody sx={stylexStyles2.rule13}>
            {rows.map((row) => (
              <tr key={row.name} sx={stylexStyles2.rule14}>
                <td sx={stylexStyles2.rule15}>{row.name}</td>
                <td sx={stylexStyles2.rule15}>{row.role}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
