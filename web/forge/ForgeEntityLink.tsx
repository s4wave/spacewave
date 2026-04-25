import { useCallback, type ReactNode } from 'react'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

interface ForgeEntityLinkProps {
  objectKey: string
  icon?: ReactNode
  label?: string
  className?: string
  children?: ReactNode
}

// ForgeEntityLink renders a clickable link that navigates to a forge entity.
// Uses SpaceContainerContext.navigateToObjects for in-space navigation.
export function ForgeEntityLink({
  objectKey,
  icon,
  label,
  className,
  children,
}: ForgeEntityLinkProps) {
  const container = SpaceContainerContext.useContextSafe()

  const onClick = useCallback(() => {
    container?.navigateToObjects([objectKey])
  }, [container, objectKey])

  if (!container) {
    return <span className={className}>{children ?? label ?? objectKey}</span>
  }

  return (
    <button
      onClick={onClick}
      className={
        className ??
        'bg-muted hover:bg-muted/70 flex w-full items-center gap-2 rounded px-3 py-1.5 text-left transition-colors'
      }
    >
      {icon}
      <span className="text-foreground flex-1 truncate font-mono text-xs">
        {children ?? label ?? objectKey}
      </span>
    </button>
  )
}
