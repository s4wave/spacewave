import { useCallback, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { LuLoaderCircle, LuPlus } from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  useSharedObjectBodyMount,
  useSharedObjectMount,
  useSpaceContentsMount,
  useSpaceMount,
} from '@s4wave/app/hooks/useSharedMounts.js'
import { useWatchSpacesList } from '@s4wave/app/system/useSystemStatus.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import {
  SpaceContentsState,
  WatchSpaceContentsStateRequest,
} from '@s4wave/sdk/space/space.pb.js'

import { knownSpacePlugin } from './known-plugins.js'

interface SourceSpace {
  id: string
  name: string
}

interface SpacePluginImportProps {
  // spaceId is the Space plugins are imported into; it is not offered as a source.
  spaceId: string
  installedIds: Set<string>
  pending: string
  onImport: (source: Space, pluginId: string) => void
}

// SpacePluginImport lists the session's other Spaces and, for the one picked,
// the plugins it stores that this Space has not installed. Adding one copies
// its builds into this Space and installs it.
export function SpacePluginImport({
  spaceId,
  installedIds,
  pending,
  onImport,
}: SpacePluginImportProps) {
  const spacesList = useWatchSpacesList()
  const [sourceId, setSourceId] = useState('')

  const sources: SourceSpace[] = []
  for (const entry of spacesList ?? []) {
    const id = entry.entry?.ref?.providerResourceRef?.id ?? ''
    if (!id || id === spaceId) continue
    sources.push({ id, name: entry.spaceMeta?.name || id })
  }
  if (sources.length === 0) return null
  const source = sources.find((space) => space.id === sourceId)

  return (
    <div className="space-y-1.5">
      <span className="text-foreground-alt/50 micro-text font-medium tracking-widest uppercase select-none">
        From another Space
      </span>
      <div className="flex flex-wrap gap-1.5">
        {sources.map((space) => (
          <button
            key={space.id}
            type="button"
            aria-pressed={space.id === sourceId}
            onClick={() => setSourceId(space.id === sourceId ? '' : space.id)}
            disabled={!!pending}
            className={cn(
              'border-foreground/8 text-foreground-alt hover:border-foreground/16 rounded-md border px-2.5 py-1 text-xs transition-colors',
              space.id === sourceId && 'border-brand/40 text-foreground',
              !!pending && 'cursor-not-allowed opacity-50',
            )}
          >
            {space.name}
          </button>
        ))}
      </div>
      {source && (
        <SourceSpaceCatalog
          key={source.id}
          source={source}
          installedIds={installedIds}
          pending={pending}
          onImport={onImport}
        />
      )}
    </div>
  )
}

interface SourceSpaceCatalogProps {
  source: SourceSpace
  installedIds: Set<string>
  pending: string
  onImport: (source: Space, pluginId: string) => void
}

// SourceSpaceCatalog mounts the source Space and lists the plugins it stores.
function SourceSpaceCatalog({
  source,
  installedIds,
  pending,
  onImport,
}: SourceSpaceCatalogProps) {
  const sessionResource = SessionContext.useContext()
  const sharedObjectResource = useSharedObjectMount(sessionResource, source.id)
  const bodyResource = useSharedObjectBodyMount(sharedObjectResource)
  const spaceResource = useSpaceMount(bodyResource)
  const sourceSpace = useResourceValue(spaceResource)
  const contentsResource = useSpaceContentsMount(spaceResource)
  const contents = useResourceValue(contentsResource)
  const contentsState = useWatchStateRpc(
    useCallback(
      (req: WatchSpaceContentsStateRequest, signal: AbortSignal) =>
        contents?.watchState(req, signal) ?? null,
      [contents],
    ),
    {},
    WatchSpaceContentsStateRequest.equals,
    SpaceContentsState.equals,
  )

  const mountError =
    sharedObjectResource.error ??
    bodyResource.error ??
    spaceResource.error ??
    contentsResource.error
  if (mountError) {
    return (
      <p className="text-destructive/80 text-xs">
        Couldn't open {source.name}: {mountError.message}
      </p>
    )
  }
  if (!sourceSpace || !contentsState) {
    return (
      <p className="text-foreground-alt/70 flex items-center gap-1.5 text-xs">
        <LuLoaderCircle className="size-3.5 animate-spin" />
        Reading the plugins in {source.name}
      </p>
    )
  }

  const plugins = (contentsState.availablePlugins ?? []).filter(
    (plugin) => plugin.pluginId && !installedIds.has(plugin.pluginId),
  )
  if (plugins.length === 0) {
    return (
      <p className="text-foreground-alt/70 text-xs">
        {source.name} has no plugin builds this Space lacks.
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-1.5">
      {plugins.map((plugin) => {
        const id = plugin.pluginId ?? ''
        const meta = knownSpacePlugin(id)
        const Icon = meta.icon
        return (
          <button
            key={id}
            type="button"
            onClick={() => onImport(sourceSpace, id)}
            disabled={!!pending}
            className={cn(
              'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-2.5 text-left transition-all duration-150',
              !!pending && 'cursor-not-allowed opacity-50',
            )}
          >
            <span className="bg-brand/10 flex size-8 shrink-0 items-center justify-center rounded-md">
              {pending === id ? (
                <LuLoaderCircle className="text-brand size-4 animate-spin" />
              ) : (
                <Icon className="text-brand size-4" />
              )}
            </span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="flex items-center gap-1.5">
                <span className="text-foreground text-xs font-medium">
                  {meta.name}
                </span>
                {plugin.revision && (
                  <span className="text-foreground-alt/50 micro-text tabular-nums">
                    rev {plugin.revision}
                  </span>
                )}
              </span>
              <span className="text-foreground-alt/70 micro-text truncate">
                {plugin.description || meta.description}
              </span>
            </span>
            <LuPlus className="text-foreground-alt/50 size-3.5 shrink-0" />
          </button>
        )
      })}
    </div>
  )
}
