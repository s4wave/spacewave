import { useCallback, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { LuPuzzle } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContentsContext } from '@s4wave/web/contexts/contexts.js'
import {
  SpaceContentsState,
  WatchSpaceContentsStateRequest,
} from '@s4wave/sdk/space/space.pb.js'
import { PluginApprovalState } from '@s4wave/core/plugin/approval/approval.pb.js'

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

  const handleApprove = useCallback(
    async (pluginId: string) => {
      await contents?.setPluginApproval(pluginId, true)
    },
    [contents],
  )

  const handleDeny = useCallback(
    async (pluginId: string) => {
      await contents?.setPluginApproval(pluginId, false)
    },
    [contents],
  )

  const [pendingDeny, setPendingDeny] = useState<string | null>(null)

  if (contentsResource.loading) {
    return (
      <div className="text-foreground-alt border-foreground/8 flex items-center justify-center rounded-lg border bg-transparent px-6 py-8 text-center">
        <p className="text-foreground-alt/50 text-xs select-none">
          Loading plugin status...
        </p>
      </div>
    )
  }

  const plugins = contentsState?.plugins ?? []

  if (plugins.length === 0) {
    return (
      <div className="text-foreground-alt border-foreground/8 flex items-center justify-center rounded-lg border bg-transparent px-6 py-8 text-center">
        <div className="text-foreground-alt">
          <LuPuzzle className="mx-auto mb-1.5 h-6 w-6 opacity-30" />
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
        const approval =
          plugin.approvalState ??
          PluginApprovalState.PluginApprovalState_UNSPECIFIED
        const desc = plugin.description ?? ''

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
              </div>
            </div>
            <div className="flex items-center gap-2">
              <ApprovalBadge state={approval} />
              {pendingDeny === id ?
                <div className="flex items-center gap-2">
                  <span className="text-foreground-alt text-xs">
                    Deny this plugin?
                  </span>
                  <button
                    className="bg-destructive/80 hover:bg-destructive rounded px-2 py-1 text-xs text-white"
                    onClick={() => {
                      void handleDeny(id).then(() => setPendingDeny(null))
                    }}
                  >
                    Confirm
                  </button>
                  <button
                    className="border-foreground/8 text-foreground hover:bg-foreground/10 rounded border px-2 py-1 text-xs"
                    onClick={() => setPendingDeny(null)}
                  >
                    Cancel
                  </button>
                </div>
              : <>
                  {approval !==
                    PluginApprovalState.PluginApprovalState_APPROVED && (
                    <button
                      className="rounded bg-green-700 px-2 py-1 text-xs text-white hover:bg-green-600"
                      onClick={() => void handleApprove(id)}
                    >
                      Approve
                    </button>
                  )}
                  {approval !==
                    PluginApprovalState.PluginApprovalState_DENIED && (
                    <button
                      className="bg-destructive/80 hover:bg-destructive rounded px-2 py-1 text-xs text-white"
                      onClick={() => setPendingDeny(id)}
                    >
                      Deny
                    </button>
                  )}
                </>
              }
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ApprovalBadge renders the approval state as a colored badge.
function ApprovalBadge({ state }: { state: PluginApprovalState }) {
  if (state === PluginApprovalState.PluginApprovalState_APPROVED) {
    return (
      <span className="rounded-full bg-green-900/50 px-2 py-0.5 text-xs text-green-400">
        Approved
      </span>
    )
  }
  if (state === PluginApprovalState.PluginApprovalState_DENIED) {
    return (
      <span className="rounded-full bg-red-900/50 px-2 py-0.5 text-xs text-red-400">
        Denied
      </span>
    )
  }
  return (
    <span className="rounded-full bg-amber-900/50 px-2 py-0.5 text-xs text-amber-400">
      Pending
    </span>
  )
}
