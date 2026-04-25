import * as React from 'react'
import AppIcon from './spacewave-icon.png'

export interface AppLogoProps extends React.ImgHTMLAttributes<HTMLImageElement> {}

export const AppLogo = React.forwardRef<HTMLImageElement, AppLogoProps>(
  (props, ref) => {
    return (
      <img
        {...props}
        ref={ref}
        style={{
          pointerEvents: 'none',
          height: 'auto',
          padding: '3.5px',
          ...props.style,
        }}
        src={AppIcon}
      />
    )
  },
)
