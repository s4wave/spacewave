import type { ReactNode } from 'react'
import type { ForgeLinkedEntity } from './useForgeLinkedEntities.js'
import { ForgeEntityLink } from './ForgeEntityLink.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

interface ForgeEntityListProps {
  entities: ForgeLinkedEntity[]
  loading: boolean
  icon: ReactNode
  loadingLabel?: string
  emptyLabel?: string
}

// ForgeEntityList renders a loading/empty/list view for linked forge entities.
export function ForgeEntityList({
  entities,
  loading,
  icon,
  loadingLabel = 'Loading linked entities',
  emptyLabel = 'None',
}: ForgeEntityListProps) {
  if (loading && entities.length === 0) {
    return (
      <div className="flex items-center justify-center py-4">
        <LoadingInline label={loadingLabel} tone="muted" size="sm" />
      </div>
    )
  }
  if (entities.length === 0) {
    return (
      <div className="text-muted-foreground py-4 text-center text-xs">
        {emptyLabel}
      </div>
    )
  }
  return (
    <div className="space-y-1">
      {entities.map((e) => (
        <ForgeEntityLink
          key={e.objectKey}
          objectKey={e.objectKey}
          icon={icon}
        />
      ))}
    </div>
  )
}
