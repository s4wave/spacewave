import * as React from 'react'
import * as stylex from '@stylexjs/stylex'

import { stylexProps } from '@s4wave/web/style/props.js'
import { layout } from '@s4wave/web/style/tokens.stylex.js'
import AppIcon from './spacewave-icon.png'

// AppLogoProps carries image attributes and the native window header styles.
export interface AppLogoProps extends Omit<
  React.ImgHTMLAttributes<HTMLImageElement>,
  'className'
> {
  topbarHeight?: number
  sx?: stylex.StyleXStyles
  className?: never
}

// AppLogo renders the Spacewave image with the platform's window header height.
export function AppLogo({
  sx,
  style,
  topbarHeight,
  ...imgProps
}: AppLogoProps) {
  const attributes = stylexProps([styles.root, sx], {
    ...imgProps,
    style:
      topbarHeight === undefined
        ? style
        : ({
            ...style,
            '--window-topbar-height': `${topbarHeight}px`,
          } as React.CSSProperties),
  })
  return <img {...attributes} src={AppIcon} alt={imgProps.alt ?? 'Spacewave'} />
}

const styles = stylex.create({
  root: {
    padding: layout['--spacing-app-logo-inset'],
    pointerEvents: 'none',
    height: 'auto',
  },
})
