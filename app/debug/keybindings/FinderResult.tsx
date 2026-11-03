import { useCallback } from 'react'
import {
  LuCheck,
  LuCommand,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import type { KeybindingCommand } from './keybindings-model.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'inline-flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule4: { flex: 1, minWidth: 0 },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    display: 'block',
    fontSize: themeLayout['--text-metadata'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: themeColors['--color-destructive'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { justifyContent: 'flex-start', textAlign: 'left', width: '100%' },
})

interface FinderResultProps {
  id: string
  command: KeybindingCommand
  selected: boolean
  hasConflict: boolean
  onSelect: (commandId: string) => void
}

export function FinderResult({
  sx,
  id,
  command,
  selected,
  hasConflict,
  onSelect,
}: FinderResultProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const select = useCallback(() => onSelect(command.id), [command.id, onSelect])

  return (
    <Button
      id={id}
      role="option"
      aria-selected={selected}
      type="button"
      tabIndex={-1}
      variant={selected ? 'selectedFinder' : 'unselectedFinder'}
      size="finderRow"
      sx={[stylexStyles.rule1, sx]}
      onClick={select}
    >
      <span
        sx={[
          stylexStyles3.rule1,
          selected ? stylexStyles3.rule2 : stylexStyles3.rule3,
        ]}
      >
        {selected ? <LuCheck /> : <LuCommand />}
      </span>
      <span sx={stylexStyles3.rule4}>
        <span sx={stylexStyles3.rule5}>
          <span sx={stylexStyles3.rule6}>{command.label}</span>
          {hasConflict ? <LuTriangleAlert sx={stylexStyles2.rule1} /> : null}
        </span>
        <span sx={stylexStyles3.rule7}>
          {command.category} · {command.context}
        </span>
      </span>
      <Keycap chord={command.binding} muted={!selected} />
    </Button>
  )
}
