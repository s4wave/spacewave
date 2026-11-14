import { useCallback } from 'react'
import { LuBug, LuCircleAlert } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { getObjectType } from '@s4wave/sdk/world/types/types.js'

import type { ObjectViewerComponentProps } from './object.js'
import { getObjectKey, getTypeID } from './object.js'
import { ViewerBar } from './ViewerBar.js'

type WorldStateValue = ObjectViewerComponentProps['worldState']['value']

// DebugObjectViewer displays debug information about an object.
// Registered as the wildcard viewer ('*'), so it also handles untyped world
// objects (objects that exist in the world graph without a type predicate).
export function DebugObjectViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  // Read the key and the type the route hints at.
  const objectKey = getObjectKey(objectInfo)
  const hintTypeID = getTypeID(objectInfo)

  // Resolve the stored type, which wins over the hint.
  const resolveTypeID = useCallback(
    async (world: WorldStateValue, signal: AbortSignal) => {
      if (!world || !objectKey) return ''
      return getObjectType(world, objectKey, signal)
    },
    [objectKey],
  )

  // Watch the resolved type in the world state.
  const resolvedTypeIDResource = useResource(worldState, resolveTypeID, [
    objectKey,
  ])
  const resolvedTypeID = resolvedTypeIDResource.value
  const resolving = resolvedTypeIDResource.loading

  // Show the resolved type, or the hint until it resolves.
  const effectiveTypeID = resolvedTypeID ?? hintTypeID
  const untyped = !resolving && !effectiveTypeID

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
      <ViewerBar icon={<LuBug />} title="Debug Viewer" />

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto max-w-3xl space-y-3">
          {untyped && (
            <div className="border-foreground/6 bg-background-card/30 flex items-start gap-3 rounded-lg border p-3.5">
              <div className="bg-foreground/5 text-foreground-alt flex size-8 shrink-0 items-center justify-center rounded-md">
                <LuCircleAlert className="size-4" />
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-foreground text-sm font-semibold tracking-tight select-none">
                  Object has no type
                </p>
                <p className="text-foreground-alt/60 mt-0.5 text-xs leading-relaxed">
                  This object exists in the world graph without a type
                  predicate, so no specialized viewer is available. The debug
                  viewer is showing raw metadata below.
                </p>
              </div>
            </div>
          )}

          <section>
            <div className="mb-2 flex items-center gap-1.5">
              <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
                <LuBug className="size-3.5" />
                Object Info
              </h3>
            </div>

            <InfoCard>
              <FactList>
                {objectKey ? (
                  <FactRow label="Object Key" value={objectKey} />
                ) : (
                  <FactRow
                    kind="text"
                    label="Object Key"
                    value="None"
                    control={null}
                  />
                )}
                {!resolving && effectiveTypeID ? (
                  <FactRow label="Type ID (resolved)" value={effectiveTypeID} />
                ) : (
                  <FactRow
                    kind="text"
                    label="Type ID (resolved)"
                    value={resolving ? 'Resolving...' : 'Untyped'}
                    control={null}
                  />
                )}
                {hintTypeID && hintTypeID !== resolvedTypeID ? (
                  <FactRow label="Type ID (route hint)" value={hintTypeID} />
                ) : null}
              </FactList>
            </InfoCard>
          </section>

          <section>
            <div className="mb-2 flex items-center gap-1.5">
              <h3 className="text-foreground text-xs font-medium select-none">
                Object State
              </h3>
            </div>

            <div className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border">
              <pre className="text-foreground-alt/80 overflow-auto p-3 font-mono text-xs leading-relaxed">
                {JSON.stringify(
                  {
                    key: objectState?.getKey() ?? null,
                    hasState: !!objectState,
                  },
                  null,
                  2,
                )}
              </pre>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
