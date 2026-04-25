import React, { useCallback } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

export interface NavigationItem {
  text: string
  textClassName?: string
  className?: string
  onClick?: () => void
}

export const NavigationLink: React.FC<NavigationItem> = ({
  text,
  textClassName,
  className,
  onClick,
}) => {
  const handleInteraction = useCallback(
    (
      e:
        | React.MouseEvent<HTMLAnchorElement>
        | React.KeyboardEvent<HTMLAnchorElement>,
    ) => {
      e.preventDefault()
      onClick?.()
    },
    [onClick],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLAnchorElement>) => {
      if (e.code === 'Space' || e.code === 'Enter' || e.code === 'Return') {
        handleInteraction(e)
      }
    },
    [handleInteraction],
  )

  return (
    <a
      href="#"
      className={cn(
        'hover:bg-navlink-selection focus:bg-navlink-selection group font-heading text-sm whitespace-nowrap transition-colors @lg:text-base',
        className,
      )}
      onClick={handleInteraction}
      onKeyDown={handleKeyDown}
    >
      <span className="text-navlink-bracket pr-navlink-bracket group-hover:no-underline group-focus:no-underline">
        [
      </span>
      <span
        className={cn(
          'text-navlink-text group-hover:text-navlink-text-hover transition-colors',
          textClassName,
        )}
      >
        {text}
      </span>
      <span className="text-navlink-bracket pl-navlink-bracket group-hover:no-underline group-focus:no-underline">
        ]
      </span>
    </a>
  )
}
