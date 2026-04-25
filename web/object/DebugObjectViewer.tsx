import { useCallback } from 'react'
import { LuBug, LuCircleAlert } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { getObjectType } from '@s4wave/sdk/world/types/types.js'

import type { ObjectViewerComponentProps } from './object.js'
import { getObjectKey, getTypeID } from './object.js'

// DebugObjectViewer displays debug information about an object.
// Registered as the wildcard viewer ('*'), so it also handles untyped world
// objects (objects that exist in the world graph without a type predicate).
export function DebugObjectViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const hintTypeID = getTypeID(objectInfo)

  const resolveTypeID = useCallback(
    async (world: typeof worldState.value, signal: AbortSignal) => {
      if (!world || !objectKey) return ''
      return getObjectType(world, objectKey, signal)
    },
    [objectKey],
  )

  const resolvedTypeIDResource = useResource(worldState, resolveTypeID, [
    objectKey,
  ])
  const resolvedTypeID = resolvedTypeIDResource.value
  const resolving = resolvedTypeIDResource.loading

  const effectiveTypeID = resolvedTypeID ?? hintTypeID
  const untyped = !resolving && !effectiveTypeID

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
          <LuBug className="h-4 w-4" />
          <span className="tracking-tight">Debug Viewer</span>
        </div>
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto max-w-3xl space-y-3">
          {untyped && (
            <div className="border-foreground/6 bg-background-card/30 flex items-start gap-3 rounded-lg border p-3.5 backdrop-blur-sm">
              <div className="bg-foreground/5 text-foreground-alt flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
                <LuCircleAlert className="h-4 w-4" />
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
              <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
                <LuBug className="h-3.5 w-3.5" />
                Object Info
              </h2>
            </div>

            <InfoCard>
              <div className="space-y-2">
                <CopyableField
                  label="Object Key"
                  value={objectKey || '(none)'}
                />
                <CopyableField
                  label="Type ID (resolved)"
                  value={
                    resolving ? 'Resolving...'
                    : effectiveTypeID ?
                      effectiveTypeID
                    : '(untyped)'
                  }
                />
                {hintTypeID && hintTypeID !== resolvedTypeID ?
                  <CopyableField
                    label="Type ID (route hint)"
                    value={hintTypeID}
                  />
                : null}
              </div>
            </InfoCard>
          </section>

          <section>
            <div className="mb-2 flex items-center gap-1.5">
              <h2 className="text-foreground text-xs font-medium select-none">
                Object State
              </h2>
            </div>

            <div className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border backdrop-blur-sm">
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
