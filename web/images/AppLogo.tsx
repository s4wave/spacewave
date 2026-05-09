import * as React from 'react'
import AppIcon from './spacewave-icon.png'

export interface AppLogoProps extends React.ImgHTMLAttributes<HTMLImageElement> {}

export function AppLogo(props: AppLogoProps) {
  return (
    <img
      {...props}
      style={{
        pointerEvents: 'none',
        height: 'auto',
        padding: '3.5px',
        ...props.style,
      }}
      src={AppIcon}
      alt={props.alt ?? 'Spacewave'}
    />
  )
}
