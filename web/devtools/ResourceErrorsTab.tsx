import { useCallback, useMemo } from 'react'
import { RiErrorWarningLine } from 'react-icons/ri'

import { cn } from '@s4wave/web/style/utils.js'
import {
  useResourceDevToolsContext,
  useTrackedResources,
  useSelectedResourceId,
  getResourceLabel,
  type TrackedResource,
} from '@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js'

// ResourceErrorsTab displays a list of resources with errors.
export function ResourceErrorsTab() {
  const resources = useTrackedResources()

  const errorResources = useMemo(() => {
    return Array.from(resources.values()).filter((r) => r.state === 'error')
  }, [resources])

  if (errorResources.length === 0) {
    return (
      <div className="text-foreground-alt/50 flex flex-1 items-center justify-center gap-1.5 p-4 text-xs">
        <RiErrorWarningLine className="h-3.5 w-3.5 opacity-50" />
        <span>No errors</span>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-auto">
      {errorResources.map((resource) => (
        <ErrorRow key={resource.id} resource={resource} />
      ))}
    </div>
  )
}

interface ErrorRowProps {
  resource: TrackedResource
}

function ErrorRow({ resource }: ErrorRowProps) {
  const devtools = useResourceDevToolsContext()
  const selectedId = useSelectedResourceId()
  const isSelected = selectedId === resource.id

  const handleClick = useCallback(() => {
    devtools?.setSelectedId(resource.id)
  }, [devtools, resource.id])

  const label = getResourceLabel(resource)

  const errorMessage = useMemo(() => {
    if (!resource.error) return ''
    const msg = resource.error.message
    const firstLine = msg.split('\n')[0]
    return firstLine.length > 50 ? firstLine.slice(0, 50) + '...' : firstLine
  }, [resource.error])

  return (
    <button
      type="button"
      onClick={handleClick}
      className={cn(
        'flex items-center gap-2 px-3 py-1.5 text-left text-xs',
        'border-popover-border/30 border-b',
        'transition-colors duration-100',
        isSelected ?
          'bg-ui-selected text-foreground'
        : 'text-text-secondary hover:bg-pulldown-hover/50',
      )}
      data-testid="resource-error-row"
    >
      <span className="bg-error h-1.5 w-1.5 shrink-0 rounded-full" />
      <span className="shrink-0 font-medium">{label}</span>
      <span className="text-foreground-alt min-w-0 flex-1 truncate font-mono">
        {errorMessage}
      </span>
    </button>
  )
}
