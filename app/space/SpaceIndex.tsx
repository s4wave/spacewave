import { useCallback, useEffect, useRef } from 'react'
import { LuBox } from 'react-icons/lu'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'

// SpaceIndex handles the root route of a space.
export function SpaceIndex() {
  const { spaceState, navigateToSubPath } = SpaceContainerContext.useContext()
  const openCommand = useOpenCommand()
  const redirectedIndexPathRef = useRef<string | null>(null)

  const indexPath = spaceState.settings?.indexPath
  const redirectPath = indexPath && indexPath !== '/' ? indexPath : null

  const handleCreateClick = useCallback(() => {
    openCommand('spacewave.create-object')
  }, [openCommand])

  useEffect(() => {
    if (!redirectPath) {
      redirectedIndexPathRef.current = null
      return
    }
    if (redirectedIndexPathRef.current === redirectPath) {
      return
    }
    redirectedIndexPathRef.current = redirectPath
    navigateToSubPath(redirectPath)
  }, [navigateToSubPath, redirectPath])

  if (redirectPath) {
    return null
  }

  return (
    <EmptyState
      className="flex-1"
      icon={<LuBox className="text-foreground-alt h-7 w-7" />}
      title="Empty Space"
      description="This space has no objects yet."
      action={{
        label: 'Create your first object',
        onClick: handleCreateClick,
      }}
    />
  )
}
