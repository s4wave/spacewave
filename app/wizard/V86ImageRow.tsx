import type { ReactNode } from 'react'

import { Button } from '@s4wave/web/ui/button.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  radii as themeRadii,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule3: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: '100%',
    justifyContent: 'flex-start',
    textAlign: 'left',
    whiteSpace: 'normal',
  },
})

// V86ImageRowProps describes one image a new VM can boot.
export interface V86ImageRowProps {
  icon: ReactNode
  title: string
  detail: string
  selected: boolean
  onSelect: () => void
}

// V86ImageRow is one image choice in the VM wizard and its catalog.
export function V86ImageRow({
  icon,
  title,
  detail,
  selected,
  onSelect,
}: V86ImageRowProps) {
  return (
    <Button
      type="button"
      aria-pressed={selected}
      variant={selected ? 'selectedRow' : 'unselectedRow'}
      size="row"
      sx={stylexStyles.rule1}
      onClick={onSelect}
    >
      <span sx={stylexStyles2.rule1}>{icon}</span>
      <span sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>{title}</span>
        <span sx={stylexStyles2.rule4}>{detail}</span>
      </span>
    </Button>
  )
}
