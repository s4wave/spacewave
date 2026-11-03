import { LuTerminal } from '@s4wave/web/images/icons.js'

import { ArchTile } from './ArchTile.js'
import { CliInstallSnippet } from './CliInstallSnippet.js'
import type { DownloadEntry, DownloadOS } from './manifest.js'
import { PrimaryDownloadButton } from './PrimaryDownloadButton.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    scrollMarginTop: `calc(${themeLayout['--spacing']} * 24)`,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-2xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-3xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule5: {
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule7: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    fontSize: {
      default: 'var(--text-lg)',
      '@container (min-width: 32rem)': 'var(--text-xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-lg--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule9: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },
})

interface CliSectionProps {
  primary: DownloadEntry | null
  groups: ReadonlyArray<{ os: DownloadOS; entries: readonly DownloadEntry[] }>
}

const OS_HEADINGS: Record<DownloadOS, string> = {
  macos: 'macOS',
  windows: 'Windows',
  linux: 'Linux',
}

// CliSection renders the standalone CLI block on the download pages.
// Layout: heading and intro, platform-aware primary download button,
// then per-OS sub-sections each containing the install snippet and the
// per-arch tile grid.
export function CliSection({ primary, groups }: CliSectionProps) {
  return (
    <section id="cli" sx={stylexStyles2.rule1} aria-labelledby="cli-heading">
      <header sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuTerminal sx={stylexStyles.rule1} />
          <h2 id="cli-heading" sx={stylexStyles2.rule4}>
            Spacewave CLI
          </h2>
        </div>
        <p sx={stylexStyles2.rule5}>
          Standalone command-line binary. Install it alongside the desktop app
          to script your workflow, drive headless servers, or pipe spacewave
          output through other Unix tools.
        </p>
      </header>

      <PrimaryDownloadButton entry={primary} />

      <div sx={stylexStyles2.rule6}>
        {groups.map(({ os, entries }) => (
          <CliPlatformGroup key={os} os={os} entries={entries} />
        ))}
      </div>
    </section>
  )
}

function CliPlatformGroup({
  os,
  entries,
}: {
  os: DownloadOS
  entries: readonly DownloadEntry[]
}) {
  if (entries.length === 0) return null
  const heading = OS_HEADINGS[os]
  const snippetEntry = entries[0]
  return (
    <div sx={[stylexStyles2.rule7]}>
      <h3 sx={stylexStyles2.rule8}>{heading}</h3>
      <CliInstallSnippet entry={snippetEntry} />
      <div sx={stylexStyles2.rule9}>
        {entries.map((entry) => (
          <ArchTile key={entry.filename} entry={entry} />
        ))}
      </div>
    </div>
  )
}
