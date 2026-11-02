import { createElement } from 'react'
import type { IconType as ForeignIconType } from 'react-icons'
import { stylexProps } from '../style/props.js'
import { useIconStyles } from './IconStylesProvider.js'
import type { IconType } from './icons.js'

/** createStyledIcon applies styles at the SVG owner while retaining the original icon context. */
export function createStyledIcon(Icon: ForeignIconType): IconType {
  function StyledIcon({ sx, ...props }: Parameters<IconType>[0]) {
    // Caller styles replace defaults through normal StyleX composition.
    const scope = useIconStyles()
    const attributes = stylexProps([scope.sx, sx], props)
    let className = attributes.className
    let style = attributes.style

    // Keep gated control rules independent so their absence cannot erase caller styles in a portal.
    for (const required of scope.required) {
      if (required.className)
        className = [className, required.className].filter(Boolean).join(' ')
      if (required.style) style = { ...style, ...required.style }
    }
    return createElement(Icon, {
      ...attributes,
      className,
      style: props.style ? { ...style, ...props.style } : style,
    })
  }
  StyledIcon.displayName = Icon.name
  return StyledIcon
}
