import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as SeparatorPrimitive from '@radix-ui/react-separator'

import * as stylex from '@stylexjs/stylex'
import { colors as themeColors } from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { flexShrink: '0', backgroundColor: themeColors['--color-border'] },
  rule2: { height: '1px', width: '100%' },
  rule3: { height: '100%', width: '1px' },
})

function Separator({
  sx,
  orientation = 'horizontal',
  decorative = true,
  ...props
}: Omit<
  React.ComponentPropsWithRef<typeof SeparatorPrimitive.Root>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <SeparatorPrimitive.Root
      decorative={decorative}
      orientation={orientation}
      {...stylex.props([
        stylexStyles.rule1,
        orientation === 'horizontal' ? stylexStyles.rule2 : stylexStyles.rule3,
        sx,
      ])}
      {...props}
    />
  )
}
Separator.displayName = SeparatorPrimitive.Root.displayName

export { Separator }
