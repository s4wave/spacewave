import { useCallback } from 'react'
import { LuTriangleAlert } from '@s4wave/web/images/icons.js'

import { Badge } from '@s4wave/web/ui/badge.js'

import { BindingCapture } from './BindingCapture.js'
import type { KeybindingCommand } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
  layout as themeLayout,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    borderStyle: {
      default: null,
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderWidth: { default: null, ':last-child': 0 },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 2%, transparent)`,
      },
    },
  },
  rule2: {
    minWidth: `calc(${themeLayout['--spacing']} * 64)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    verticalAlign: 'middle',
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
  },
  rule6: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule7: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    verticalAlign: 'middle',
  },
  rule8: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule9: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'right',
    verticalAlign: 'middle',
  },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

interface CommandTableRowProps {
  command: KeybindingCommand
  hasConflict: boolean
  isCustomized: boolean
  setBinding: KeybindingVariantProps['setBinding']
  resetBinding: KeybindingVariantProps['resetBinding']
}

export function CommandTableRow({
  command,
  hasConflict,
  isCustomized,
  setBinding,
  resetBinding,
}: CommandTableRowProps) {
  const changeBinding = useCallback(
    (binding: string) => setBinding(command.id, binding),
    [command.id, setBinding],
  )
  const reset = useCallback(
    () => resetBinding(command.id),
    [command.id, resetBinding],
  )

  return (
    <tr sx={stylexStyles.rule1}>
      <td sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <span sx={stylexStyles.rule4}>{command.label}</span>
          {isCustomized ? (
            <span sx={stylexStyles.rule5} title="Customized" />
          ) : null}
        </div>
        <p sx={stylexStyles.rule6}>{command.description}</p>
      </td>
      <td sx={stylexStyles.rule7}>
        <div sx={stylexStyles.rule8}>
          <Badge variant="muted">{command.category}</Badge>
          <Badge variant="subtle">{command.context}</Badge>
        </div>
      </td>
      <td sx={stylexStyles.rule9}>
        <div sx={stylexStyles.rule10}>
          {hasConflict ? (
            <Badge variant="conflict">
              <LuTriangleAlert /> Conflict
            </Badge>
          ) : null}
          <BindingCapture
            binding={command.binding}
            commandLabel={command.label}
            isCustomized={isCustomized}
            onChange={changeBinding}
            onReset={reset}
          />
        </div>
      </td>
    </tr>
  )
}
