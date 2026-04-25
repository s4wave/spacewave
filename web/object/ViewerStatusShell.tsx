import type { ReactNode } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

// ViewerStatusShellProps configures the loading, error, and empty state messages.
interface ViewerStatusShellProps {
  resource: Resource<unknown>
  state: Resource<unknown>
  loadingText: string
  emptyText?: string
  sources?: unknown[]
  children: ReactNode
}

// ViewerStatusShell renders loading, error, and optionally empty states
// with consistent styling. Renders children when the resource is ready.
export function ViewerStatusShell({
  resource,
  state,
  loadingText,
  emptyText,
  sources,
  children,
}: ViewerStatusShellProps) {
  if (resource.loading || state.loading) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center">
        {loadingText}
      </div>
    )
  }

  if (resource.error) {
    return (
      <div className="text-destructive flex h-full items-center justify-center">
        {resource.error.message}
      </div>
    )
  }

  if (emptyText && sources !== undefined && sources.length === 0) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        {emptyText}
      </div>
    )
  }

  return <>{children}</>
}
