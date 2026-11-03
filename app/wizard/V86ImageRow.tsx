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
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: 'var(--leading-relaxed)',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    justifyContent: 'flex-start',
    textAlign: 'left',
    whiteSpace: 'normal',
    width: '100%',
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
