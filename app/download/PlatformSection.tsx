import { ArchTile } from './ArchTile.js'
import type { DownloadEntry, DownloadOS } from './manifest.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
  },
  rule3: {
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
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule5: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
})

interface PlatformSectionProps {
  os: DownloadOS
  entries: readonly DownloadEntry[]
}

const OS_HEADINGS: Record<DownloadOS, string> = {
  macos: 'macOS',
  windows: 'Windows',
  linux: 'Linux',
}

// PlatformSection groups per-architecture download tiles under a single OS
// heading. Windows entries carrying unsigned: true surface an "Interim
// unsigned build" caption under the heading; the SmartScreen bypass details
// render elsewhere (WindowsBypassNotice).
export function PlatformSection({ os, entries }: PlatformSectionProps) {
  const heading = OS_HEADINGS[os]
  const isInterimUnsigned = os === 'windows' && entries.some((e) => e.unsigned)

  return (
    <section sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <h2 sx={stylexStyles.rule3}>{heading}</h2>
        {isInterimUnsigned && (
          <p sx={stylexStyles.rule4}>Interim unsigned build</p>
        )}
      </div>
      <div sx={stylexStyles.rule5}>
        {entries.map((entry) => (
          <ArchTile key={entry.filename} entry={entry} />
        ))}
      </div>
    </section>
  )
}
