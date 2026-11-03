import { useCallback } from 'react'
import { LuKey } from '@s4wave/web/images/icons.js'

import { EmptyState } from '@s4wave/web/ui/EmptyState.js'

import type { KvKeyRow } from './kv-viewer.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { flex: 1, overflow: 'auto' },
  rule2: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule4: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule5: {
    flexShrink: 0,
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },
})

interface KvKeyListProps {
  rows: KvKeyRow[]
  selectedKey: string | null
  onSelectKey: (row: KvKeyRow) => void
  filtered: boolean
}

// KvKeyList renders the scrollable key list with a byte-length badge per row.
export function KvKeyList({
  rows,
  selectedKey,
  onSelectKey,
  filtered,
}: KvKeyListProps) {
  if (rows.length === 0) {
    return (
      <div sx={stylexStyles2.rule1}>
        <EmptyState
          variant="compact"
          icon={<LuKey sx={stylexStyles.rule1} />}
          title={filtered ? 'No matching keys' : 'No keys yet'}
          description={
            filtered
              ? 'No keys match this prefix.'
              : 'Add a key to populate this store.'
          }
        />
      </div>
    )
  }

  return (
    <div role="listbox" aria-label="Keys" sx={stylexStyles2.rule1}>
      {rows.map((row) => (
        <KvKeyListRow
          key={row.label}
          row={row}
          selected={row.label === selectedKey}
          onSelectKey={onSelectKey}
        />
      ))}
    </div>
  )
}

interface KvKeyListRowProps {
  row: KvKeyRow
  selected: boolean
  onSelectKey: (row: KvKeyRow) => void
}

function KvKeyListRow({ row, selected, onSelectKey }: KvKeyListRowProps) {
  const selectKey = useCallback(() => {
    onSelectKey(row)
  }, [onSelectKey, row])

  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      onClick={selectKey}
      sx={selected ? stylexStyles2.rule2 : stylexStyles2.rule3}
    >
      <span sx={stylexStyles2.rule4}>{row.label}</span>
      <span sx={stylexStyles2.rule5}>{row.byteLength} B</span>
    </button>
  )
}
