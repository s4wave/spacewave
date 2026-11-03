import { useCallback } from 'react'
import {
  LuCheck,
  LuCircleOff,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import type { KeybindingCommand } from './keybindings-model.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    display: 'inline-flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 12%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule5: { flex: 1, minWidth: 0 },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule8: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    display: 'block',
    fontSize: '10px',
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: 1.5,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { justifyContent: 'flex-start', textAlign: 'left', width: '100%' },
})

interface AuditQueueRowProps {
  command: KeybindingCommand
  selected: boolean
  hasConflict: boolean
  isCustomized: boolean
  onSelect: (commandId: string) => void
}

export function AuditQueueRow({
  sx,
  command,
  selected,
  hasConflict,
  isCustomized,
  onSelect,
}: AuditQueueRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const select = useCallback(() => onSelect(command.id), [command.id, onSelect])

  return (
    <Button
      type="button"
      aria-pressed={selected}
      variant={selected ? 'selectedRow' : 'unselectedRow'}
      size="row"
      sx={[stylexStyles.rule1, sx]}
      onClick={select}
    >
      <span
        sx={[
          stylexStyles2.rule1,
          hasConflict
            ? stylexStyles2.rule2
            : command.binding
              ? stylexStyles2.rule3
              : stylexStyles2.rule4,
        ]}
      >
        {hasConflict ? (
          <LuTriangleAlert />
        ) : command.binding ? (
          <LuCheck />
        ) : (
          <LuCircleOff />
        )}
      </span>
      <span sx={stylexStyles2.rule5}>
        <span sx={stylexStyles2.rule6}>
          <span sx={stylexStyles2.rule7}>{command.label}</span>
          {isCustomized ? <span sx={stylexStyles2.rule8} /> : null}
        </span>
        <span sx={stylexStyles2.rule9}>
          {hasConflict
            ? 'Conflicting assignment'
            : command.binding
              ? `${command.category} · ${command.context}`
              : 'No shortcut assigned'}
        </span>
      </span>
      <Keycap chord={command.binding} muted={!selected} />
    </Button>
  )
}
