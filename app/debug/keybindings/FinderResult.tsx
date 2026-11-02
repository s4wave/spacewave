import { useCallback } from 'react'
import {
  LuCheck,
  LuCommand,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import type { KeybindingCommand } from './keybindings-model.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

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
  id,
  command,
  selected,
  hasConflict,
  onSelect,
}: FinderResultProps) {
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
      sx={stylexStyles.rule1}
      onClick={select}
    >
      <span
        className={cn(
          'inline-flex size-7 shrink-0 items-center justify-center rounded-md',
          selected
            ? 'bg-brand/15 text-brand'
            : 'bg-foreground/5 text-foreground-alt/50',
        )}
      >
        {selected ? <LuCheck /> : <LuCommand />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{command.label}</span>
          {hasConflict ? <LuTriangleAlert sx={stylexStyles2.rule1} /> : null}
        </span>
        <span className="text-foreground-alt/45 text-metadata mt-0.5 block truncate font-normal">
          {command.category} · {command.context}
        </span>
      </span>
      <Keycap chord={command.binding} muted={!selected} />
    </Button>
  )
}
