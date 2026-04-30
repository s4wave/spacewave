import { useCallback, type ReactNode } from 'react'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import type { ForgeLinkedEntity } from './useForgeLinkedEntities.js'

interface ForgeEntityListProps {
  entities: ForgeLinkedEntity[]
  loading: boolean
  icon: ReactNode
  loadingLabel?: string
  emptyLabel?: string
}

// ForgeEntityList renders a compact dense list of forge entities with a
// loading inline spinner and a compact single-line empty state.
export function ForgeEntityList({
  entities,
  loading,
  icon,
  loadingLabel = 'Loading linked entities',
  emptyLabel = 'None linked yet',
}: ForgeEntityListProps) {
  if (loading && entities.length === 0) {
    return (
      <div className="flex items-center justify-center py-3">
        <LoadingInline label={loadingLabel} tone="muted" size="sm" />
      </div>
    )
  }
  if (entities.length === 0) {
    return (
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
          <span className="shrink-0">{icon}</span>
          <span>{emptyLabel}</span>
        </div>
      </div>
    )
  }
  return (
    <div className="border-foreground/6 bg-background-card/30 divide-foreground/6 divide-y rounded-lg border">
      {entities.map((entity) => (
        <DenseEntityRow key={entity.objectKey} entity={entity} icon={icon} />
      ))}
    </div>
  )
}

interface DenseEntityRowProps {
  entity: ForgeLinkedEntity
  icon: ReactNode
}

// DenseEntityRow renders a single compact row (icon + key + navigate-on-click).
function DenseEntityRow({ entity, icon }: DenseEntityRowProps) {
  const container = SpaceContainerContext.useContextSafe()
  const onClick = useCallback(() => {
    container?.navigateToObjects([entity.objectKey])
  }, [container, entity.objectKey])
  const className =
    'hover:bg-foreground/5 flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors first:rounded-t-lg last:rounded-b-lg'
  if (!container) {
    return (
      <div className={className}>
        <span className="text-foreground-alt/60 shrink-0">{icon}</span>
        <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
          {entity.objectKey}
        </span>
      </div>
    )
  }
  return (
    <button type="button" onClick={onClick} className={className}>
      <span className="text-foreground-alt/60 shrink-0">{icon}</span>
      <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
        {entity.objectKey}
      </span>
    </button>
  )
}
