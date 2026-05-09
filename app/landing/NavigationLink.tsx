import { useCallback } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

export interface NavigationItem {
  text: string
  textClassName?: string
  className?: string
  onClick?: () => void
}

export function NavigationLink({
  text,
  textClassName,
  className,
  onClick,
}: NavigationItem) {
  const handleNavigationSelect = useCallback(() => onClick?.(), [onClick])

  return (
    <button
      type="button"
      className={cn(
        'hover:bg-navlink-selection focus:bg-navlink-selection group font-heading bg-transparent p-0 text-sm whitespace-nowrap transition-colors @lg:text-base',
        className,
      )}
      onClick={handleNavigationSelect}
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
    </button>
  )
}
