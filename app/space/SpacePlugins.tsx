import { type ComponentType, useCallback, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { LuLoaderCircle, LuPlus, LuPuzzle, LuTrash2, LuX } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  SpaceContentsContext,
  SpaceContext,
} from '@s4wave/web/contexts/contexts.js'
import { PluginLifecycleBadge } from '@s4wave/web/sdk/app/lifecycle.js'
import { isValidSpacePluginId } from '@s4wave/core/space/world/world.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { Input } from '@s4wave/web/ui/input.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import {
  SpaceContentsState,
  WatchSpaceContentsStateRequest,
} from '@s4wave/sdk/space/space.pb.js'

import { KNOWN_SPACE_PLUGINS, knownSpacePlugin } from './known-plugins.js'

// CatalogEntry is one browsable plugin row merging the backend catalog with
// known-plugin presentation metadata (name, icon, fallback description).
interface CatalogEntry {
  id: string
  name: string
  description: string
  icon: ComponentType<{ className?: string }>
  revision: string
}

// SpacePlugins renders the plugin management UI for a space: the installed
// plugins with lifecycle badges, an add-by-manifest-ID flow with known-plugin
// suggestions, and a per-row remove with inline confirmation.
export function SpacePlugins() {
  const spaceResource = SpaceContext.useContext()
  const space = useResourceValue(spaceResource)
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

  const plugins = contentsState?.plugins ?? []
  const installedIds = new Set(plugins.map((plugin) => plugin.pluginId ?? ''))

  const [adding, setAdding] = useState(false)
  const [draftId, setDraftId] = useState('')
  // pending holds the plugin ID of an in-flight add/remove mutation so its row
  // and control disable while the RPC runs.
  const [pending, setPending] = useState('')
  // confirmingRemoveId holds the plugin ID whose remove row is showing the
  // inline confirm affordance.
  const [confirmingRemoveId, setConfirmingRemoveId] = useState('')

  const handleAdd = useCallback(
    async (pluginId: string) => {
      if (!space || pending) return
      setPending(pluginId)
      try {
        await space.addSpacePlugin(pluginId)
        setDraftId('')
        setAdding(false)
      } catch (err) {
        toast.error('Failed to add plugin', { description: String(err) })
      } finally {
        setPending('')
      }
    },
    [space, pending],
  )

  const handleRemove = useCallback(
    async (pluginId: string) => {
      if (!space || pending) return
      setPending(pluginId)
      try {
        await space.removeSpacePlugin(pluginId)
        setConfirmingRemoveId('')
      } catch (err) {
        toast.error('Failed to remove plugin', { description: String(err) })
      } finally {
        setPending('')
      }
    },
    [space, pending],
  )

  const trimmedDraft = draftId.trim()
  const draftValid = isValidSpacePluginId(trimmedDraft)
  const draftDuplicate = installedIds.has(trimmedDraft)
  const draftError =
    trimmedDraft.length === 0
      ? null
      : draftDuplicate
        ? 'Plugin already installed'
        : !draftValid
          ? 'Use a lowercase manifest ID (letters, digits, dashes)'
          : null
  const canSubmitDraft = draftValid && !draftDuplicate && !pending

  // Merge the backend plugin catalog with known-plugin presentation metadata,
  // keyed by manifest ID. The streamed availablePlugins list is the source of
  // truth for what is installable; known-plugins supplies name/icon and a
  // fallback description, and stands in as the browsable set until the catalog
  // syncs.
  const available = contentsState?.availablePlugins ?? []
  const catalogById = new Map<string, CatalogEntry>()
  for (const known of KNOWN_SPACE_PLUGINS) {
    catalogById.set(known.id, {
      id: known.id,
      name: known.name,
      description: known.description,
      icon: known.icon,
      revision: '',
    })
  }
  for (const plugin of available) {
    const id = plugin.pluginId ?? ''
    if (!id) continue
    const meta = knownSpacePlugin(id)
    catalogById.set(id, {
      id,
      name: meta.name,
      description: plugin.description || meta.description,
      icon: meta.icon,
      revision: plugin.revision ?? '',
    })
  }
  const suggestions = Array.from(catalogById.values())
    .filter((entry) => !installedIds.has(entry.id))
    .sort((a, b) => a.name.localeCompare(b.name))

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <span className="text-foreground-alt/50 text-[0.6rem] font-medium tracking-widest uppercase select-none">
          {plugins.length} installed
        </span>
        <DashboardButton
          icon={
            adding ? (
              <LuX className="size-3.5" />
            ) : (
              <LuPlus className="size-3.5" />
            )
          }
          onClick={() => {
            setAdding((open) => !open)
            setDraftId('')
          }}
          aria-label={adding ? 'Cancel adding plugin' : 'Add plugin'}
        >
          {adding ? 'Cancel' : 'Add'}
        </DashboardButton>
      </div>

      {adding && (
        <div className="border-foreground/6 bg-background-card/30 space-y-3 rounded-lg border p-3.5">
          <div className="space-y-1.5">
            <label
              htmlFor="space-plugin-id"
              className="text-foreground text-xs font-medium select-none"
            >
              Plugin manifest ID
            </label>
            <div className="flex items-center gap-2">
              <Input
                id="space-plugin-id"
                value={draftId}
                onChange={(e) => setDraftId(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && canSubmitDraft) {
                    void handleAdd(trimmedDraft)
                  }
                }}
                aria-invalid={draftError != null}
                placeholder="spacewave-notes"
                disabled={!!pending}
                className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-8 flex-1 font-mono text-xs"
              />
              <Button
                type="button"
                size="sm"
                onClick={() => void handleAdd(trimmedDraft)}
                disabled={!canSubmitDraft}
                className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-8 rounded-md border px-3 text-xs"
              >
                {pending !== '' && pending === trimmedDraft ? (
                  <LuLoaderCircle className="size-3.5 animate-spin" />
                ) : (
                  'Add'
                )}
              </Button>
            </div>
            {draftError && (
              <p className="text-destructive/80 text-[0.6rem]">{draftError}</p>
            )}
          </div>

          {suggestions.length > 0 && (
            <div className="space-y-1.5">
              <span className="text-foreground-alt/50 text-[0.6rem] font-medium tracking-widest uppercase select-none">
                Available plugins
              </span>
              <div className="flex flex-col gap-1.5">
                {suggestions.map((plugin) => {
                  const Icon = plugin.icon
                  const isPending = pending === plugin.id
                  return (
                    <button
                      key={plugin.id}
                      type="button"
                      onClick={() => void handleAdd(plugin.id)}
                      disabled={!!pending}
                      className={cn(
                        'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-2.5 text-left transition-all duration-150',
                        !!pending && 'cursor-not-allowed opacity-50',
                      )}
                    >
                      <span className="bg-brand/10 flex size-8 shrink-0 items-center justify-center rounded-md">
                        {isPending ? (
                          <LuLoaderCircle className="text-brand size-4 animate-spin" />
                        ) : (
                          <Icon className="text-brand size-4" />
                        )}
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="flex items-center gap-1.5">
                          <span className="text-foreground text-xs font-medium">
                            {plugin.name}
                          </span>
                          {plugin.revision && (
                            <span className="text-foreground-alt/50 text-[0.6rem] tabular-nums">
                              rev {plugin.revision}
                            </span>
                          )}
                        </span>
                        <span className="text-foreground-alt/70 truncate text-[0.6rem]">
                          {plugin.description}
                        </span>
                      </span>
                      <LuPlus className="text-foreground-alt/50 size-3.5 shrink-0" />
                    </button>
                  )
                })}
              </div>
            </div>
          )}
        </div>
      )}

      {plugins.length === 0 ? (
        <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
          <LuPuzzle className="size-3.5 shrink-0" />
          <span className="select-none">No plugins installed</span>
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {plugins.map((plugin) => {
            const id = plugin.pluginId ?? ''
            const meta = knownSpacePlugin(id)
            const desc = plugin.description || meta.description
            const detail = plugin.detail ?? ''
            const confirming = confirmingRemoveId === id
            const isPending = pending === id

            if (confirming) {
              return (
                <div
                  key={id}
                  className="border-destructive/30 bg-destructive/5 flex items-center justify-between gap-2 rounded-lg border px-3 py-2"
                >
                  <span className="text-destructive/90 min-w-0 flex-1 truncate text-xs select-none">
                    Remove {id}?
                  </span>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => void handleRemove(id)}
                      disabled={isPending}
                    >
                      {isPending ? (
                        <LuLoaderCircle className="size-3.5 animate-spin" />
                      ) : (
                        'Confirm'
                      )}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setConfirmingRemoveId('')}
                      disabled={isPending}
                    >
                      Cancel
                    </Button>
                  </div>
                </div>
              )
            }

            return (
              <div
                key={id}
                className="border-foreground/8 flex items-center justify-between gap-2 rounded-lg border bg-transparent px-3 py-2"
              >
                <div className="flex min-w-0 flex-col">
                  <span className="text-foreground truncate text-sm">{id}</span>
                  {desc && (
                    <span className="text-foreground-alt truncate text-xs">
                      {desc}
                    </span>
                  )}
                  {detail && (
                    <span className="text-foreground-alt/70 truncate text-xs">
                      {detail}
                    </span>
                  )}
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <PluginLifecycleBadge plugin={plugin} />
                  <button
                    type="button"
                    onClick={() => setConfirmingRemoveId(id)}
                    disabled={!!pending}
                    aria-label={`Remove ${id}`}
                    className={cn(
                      'text-foreground-alt/50 hover:text-destructive flex size-6 items-center justify-center rounded-md transition-colors',
                      !!pending && 'cursor-not-allowed opacity-50',
                    )}
                  >
                    <LuTrash2 className="size-3.5" />
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
