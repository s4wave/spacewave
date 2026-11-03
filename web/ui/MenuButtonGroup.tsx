import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { type ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({ rule1: { gap: '1px', display: 'flex' } })

interface MenuButtonGroupProps {
  className?: never
  children: ReactNode
  sx?: ComponentStyles
}

export function MenuButtonGroup({
  children,
  sx,
}: Omit<MenuButtonGroupProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return <div sx={[stylexStyles.rule1, sx]}>{children}</div>
}
