import * as React from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import AppIcon from './spacewave-icon.png'

export interface AppLogoProps extends React.ImgHTMLAttributes<HTMLImageElement> {
  topbarHeight?: number
}

export function AppLogo(props: AppLogoProps) {
  const { className, style: _style, topbarHeight, ...imgProps } = props
  return (
    <img
      {...imgProps}
      className={cn('pointer-events-none h-auto p-app-logo-inset', className)}
      style={
        topbarHeight === undefined
          ? undefined
          : { '--window-topbar-height': `${topbarHeight}px` }
      }
      src={AppIcon}
      alt={props.alt ?? 'Spacewave'}
    />
  )
}
