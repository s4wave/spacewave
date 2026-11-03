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
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    minWidth: 0,
    width: '100%',
  },
  rule2: { alignItems: 'flex-end' },
  rule3: {
    borderRadius: 'var(--radius-2xl)',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: themeColors['--color-muted'],
    color: themeColors['--color-foreground'],
    overflowWrap: 'break-word',
    maxWidth: '100%',
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    overflowWrap: 'break-word',
    minWidth: 0,
    width: '100%',
  },
  rule5: { whiteSpace: 'pre-wrap' },
  rule6: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
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
