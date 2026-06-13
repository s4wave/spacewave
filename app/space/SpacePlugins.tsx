import { useCallback } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { LuPuzzle } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContentsContext } from '@s4wave/web/contexts/contexts.js'
import { PluginLifecycleBadge } from '@s4wave/web/sdk/app/lifecycle.js'
import {
  SpaceContentsState,
  WatchSpaceContentsStateRequest,
} from '@s4wave/sdk/space/space.pb.js'

// SpacePlugins renders the plugin management UI for a space.
export function SpacePlugins() {
  const contentsResource = SpaceContentsContext.useContext()
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

  if (contentsResource.loading) {
    return (
      <div className="text-foreground-alt border-foreground/8 flex items-center justify-center rounded-lg border bg-transparent px-6 py-8 text-center">
        <p className="text-foreground-alt/50 text-xs select-none">
          Loading plugin status…
        </p>
      </div>
    )
  }

  const plugins = contentsState?.plugins ?? []

  if (plugins.length === 0) {
    return (
      <div className="text-foreground-alt border-foreground/8 flex items-center justify-center rounded-lg border bg-transparent px-6 py-8 text-center">
        <div className="text-foreground-alt">
          <LuPuzzle className="mx-auto mb-1.5 size-6 opacity-30" />
          <p className="text-foreground-alt/50 text-xs select-none">
            No plugins configured
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {plugins.map((plugin) => {
        const id = plugin.pluginId ?? ''
        const desc = plugin.description ?? ''
        const detail = plugin.detail ?? ''

        return (
          <div
            key={id}
            className="border-foreground/8 flex items-center justify-between rounded-lg border bg-transparent px-3 py-2"
          >
            <div className="flex items-center gap-3">
              <div className="flex flex-col">
                <span className="text-foreground text-sm">{id}</span>
                {desc && (
                  <span className="text-foreground-alt text-xs">{desc}</span>
                )}
                {detail && (
                  <span className="text-foreground-alt/70 text-xs">
                    {detail}
                  </span>
                )}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <PluginLifecycleBadge plugin={plugin} />
            </div>
          </div>
        )
      })}
    </div>
  )
}
