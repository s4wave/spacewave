import * as ownerStylex from '@stylexjs/stylex'
import { cn as ownerClasses } from '@s4wave/web/style/utils.js'
import { groupMarker as ownergroupMarker } from '@s4wave/web/style/relations.stylex.js'
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
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          ownerClasses(
            'hover:bg-navlink-selection focus:bg-navlink-selection group font-heading bg-transparent p-0 text-sm whitespace-nowrap transition-colors @lg:text-base',
            ownerStylex.props(ownergroupMarker).className,
          ),
          className,
        )
      }
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
