import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useCallback } from 'react'

import { OrgHandle, OrganizationTypeID } from '@s4wave/sdk/org/org.js'
import type { OrgState } from '@s4wave/sdk/org/org.pb.js'

export { OrganizationTypeID }

// OrgViewer displays organization state from the typed resource.
export function OrgViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    OrgHandle,
    OrganizationTypeID,
  )

  const streamFactory = useCallback(
    (h: OrgHandle, signal: AbortSignal) => h.watchOrgState(signal),
    [],
  )

  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: OrgState | undefined = stateResource.value ?? undefined

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Organization
        </span>
      </div>
      <div className="flex-1 p-4">
        {stateResource.loading && !state && (
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading organization',
              detail: 'Reading the organization state stream.',
            }}
          />
        )}
        {state && (
          <div className="space-y-2">
            <div className="text-foreground text-lg font-semibold">
              {state.displayName || 'Untitled Organization'}
            </div>
            <div className="text-muted-foreground text-sm">
              {state.members?.length ?? 0} member
              {(state.members?.length ?? 0) !== 1 ? 's' : ''}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
