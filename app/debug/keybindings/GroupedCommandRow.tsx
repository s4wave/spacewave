import { useCallback } from 'react'
import { LuTriangleAlert } from '@s4wave/web/images/icons.js'

import { Badge } from '@s4wave/web/ui/badge.js'

import { BindingCapture } from './BindingCapture.js'
import type { KeybindingCommand } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    alignItems: { default: null, '@media (min-width: 40rem)': 'center' },
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule2: { minWidth: 0 },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    overflow: 'hidden',
    WebkitBoxOrient: 'vertical',
    WebkitLineClamp: 1,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: '-webkit-box',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule6: { flexShrink: 0 },
})

interface GroupedCommandRowProps {
  command: KeybindingCommand
  hasConflict: boolean
  isCustomized: boolean
  setBinding: KeybindingVariantProps['setBinding']
  resetBinding: KeybindingVariantProps['resetBinding']
}

export function GroupedCommandRow({
  command,
  hasConflict,
  isCustomized,
  setBinding,
  resetBinding,
}: GroupedCommandRowProps) {
  const changeBinding = useCallback(
    (binding: string) => setBinding(command.id, binding),
    [command.id, setBinding],
  )
  const reset = useCallback(
    () => resetBinding(command.id),
    [command.id, resetBinding],
  )

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <span sx={stylexStyles.rule4}>{command.label}</span>
          <Badge variant="muted">{command.context}</Badge>
          {hasConflict ? (
            <Badge variant="conflict">
              <LuTriangleAlert /> Conflict
            </Badge>
          ) : null}
        </div>
        <p sx={stylexStyles.rule5}>{command.description}</p>
      </div>
      <BindingCapture
        binding={command.binding}
        commandLabel={command.label}
        isCustomized={isCustomized}
        sx={stylexStyles.rule6}
        onChange={changeBinding}
        onReset={reset}
      />
    </div>
  )
}
