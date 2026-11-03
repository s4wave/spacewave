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
    display: 'inline-flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule4: { minWidth: 0, flex: 1 },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles = stylex.create({
  rule1: { width: '100%', justifyContent: 'flex-start', textAlign: 'left' },
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
