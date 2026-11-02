import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    minWidth: 0,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: { alignItems: 'flex-end' },
  rule3: {
    maxWidth: '100%',
    borderRadius: 'var(--radius-2xl)',
    backgroundColor: themeColors['--color-muted'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    overflowWrap: 'break-word',
    color: themeColors['--color-foreground'],
  },
  rule4: {
    display: 'flex',
    width: '100%',
    minWidth: 0,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    overflowWrap: 'break-word',
  },
  rule5: { whiteSpace: 'pre-wrap' },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule7: { justifyContent: 'flex-end' },
})

/** MessageFrom is who sent the turn. */
export type MessageFrom = 'user' | 'assistant'

/** Message renders one turn. The person's text sits in a bubble on the right. */
function Message({
  from,
  text,
  actions,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    from: MessageFrom
    text: string
    actions?: React.ReactNode
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const alignEnd = from === 'user'

  return (
    <div
      data-slot="message"
      {...stylexProps(
        [stylexStyles.rule1, !!alignEnd && stylexStyles.rule2, sx],
        props,
      )}
    >
      {alignEnd ? (
        <div data-slot="message-bubble" sx={stylexStyles.rule3}>
          {text}
        </div>
      ) : (
        <div data-slot="message-content" sx={stylexStyles.rule4}>
          <p sx={stylexStyles.rule5}>{text}</p>
        </div>
      )}
      {actions ? (
        <div
          data-slot="message-footer"
          sx={[stylexStyles.rule6, !!alignEnd && stylexStyles.rule7]}
        >
          {actions}
        </div>
      ) : null}
    </div>
  )
}

export { Message }
