import { useCallback, useState } from 'react'
import { LuActivity } from 'react-icons/lu'

import type { BackgroundPlugin } from '@s4wave/core/session/session.pb.js'
import { knownSpacePlugin } from '@s4wave/app/space/known-plugins.js'
import { useWatchSpacesList } from '@s4wave/app/system/useSystemStatus.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import {
  type BackgroundPlugins,
  useBackgroundPlugins,
} from './useBackgroundPlugins.js'

// BackgroundSpace groups the background plugins of one Space.
interface BackgroundSpace {
  spaceId: string
  name: string
  plugins: BackgroundPlugin[]
}

// groupBySpace groups entries by Space in their saved order, naming each Space
// from names.
function groupBySpace(
  entries: BackgroundPlugin[],
  names: Map<string, string>,
): BackgroundSpace[] {
  const spaces: BackgroundSpace[] = []
  for (const entry of entries) {
    const spaceId = entry.spaceId ?? ''
    let space = spaces.find((space) => space.spaceId === spaceId)
    if (!space) {
      space = {
        spaceId,
        name: names.get(spaceId) || 'Untitled space',
        plugins: [],
      }
      spaces.push(space)
    }
    space.plugins.push(entry)
  }
  return spaces
}

// SessionBackgroundPluginsButton registers the bottom-bar item that lists the
// plugins confirmed to run while their Space is closed. It shows only while
// the Session has one.
export function SessionBackgroundPluginsButton() {
  const backgroundPlugins = useBackgroundPlugins()
  const entries = backgroundPlugins.entries
  const running = entries.filter((entry) => !entry.suspended).length
  const ariaLabel =
    running === 0
      ? 'Background plugins: all suspended'
      : `Background plugins: ${running} running`
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <Popover open={selected}>
        <PopoverAnchor asChild>
          <BottomBarItem
            selected={selected}
            onClick={onClick}
            className={cn(className, running !== 0 && 'text-brand')}
            aria-label={ariaLabel}
            data-testid="session-background-plugins-button"
          >
            <LuActivity className="size-3.5" aria-hidden="true" />
          </BottomBarItem>
        </PopoverAnchor>
        <PopoverContent
          side="top"
          align="end"
          sideOffset={6}
          onEscapeKeyDown={onClick}
          onPointerDownOutside={onClick}
          variant="status"
        >
          <SessionBackgroundPluginsPopover
            backgroundPlugins={backgroundPlugins}
          />
        </PopoverContent>
      </Popover>
    ),
    [running, ariaLabel, backgroundPlugins],
  )

  if (entries.length === 0) return null

  return (
    <BottomBarLevel
      id="session-background-plugins"
      position="right"
      button={buttonRender}
    >
      {null}
    </BottomBarLevel>
  )
}

// SessionBackgroundPluginsPopover lists each Space with its background
// plugins and their Suspend and Resume controls.
function SessionBackgroundPluginsPopover({
  backgroundPlugins,
}: {
  backgroundPlugins: BackgroundPlugins
}) {
  const spacesList = useWatchSpacesList()
  const [busyKey, setBusyKey] = useState('')
  const [actionError, setActionError] = useState('')

  // Name each Space from the Session's Space list.
  const names = new Map<string, string>()
  for (const entry of spacesList ?? []) {
    const id = entry.entry?.ref?.providerResourceRef?.id
    if (id) names.set(id, entry.spaceMeta?.name ?? '')
  }
  const spaces = groupBySpace(backgroundPlugins.entries, names)

  const handleToggle = useCallback(
    async (entry: BackgroundPlugin) => {
      const key = `${entry.spaceId}/${entry.pluginId}`
      if (busyKey) return
      setBusyKey(key)
      setActionError('')
      try {
        await backgroundPlugins.set(
          entry.spaceId ?? '',
          entry.pluginId ?? '',
          true,
          !entry.suspended,
        )
      } catch (err) {
        setActionError(err instanceof Error ? err.message : String(err))
      } finally {
        setBusyKey('')
      }
    },
    [busyKey, backgroundPlugins],
  )

  return (
    <div
      className="space-y-3 p-3"
      data-testid="session-background-plugins-popover"
    >
      <div>
        <div className="text-sm font-semibold tracking-tight">
          Background plugins
        </div>
        <div className="text-foreground-alt/60 mt-0.5 text-xs">
          These keep running while their Space is closed.
        </div>
      </div>

      {spaces.map((space) => (
        <div
          key={space.spaceId}
          className="border-foreground/8 space-y-1.5 border-t pt-2"
        >
          <div className="text-foreground-alt/50 micro-text truncate font-medium tracking-widest uppercase">
            {space.name}
          </div>
          {space.plugins.map((entry) => {
            const key = `${entry.spaceId}/${entry.pluginId}`
            const name = knownSpacePlugin(entry.pluginId ?? '').name
            return (
              <div
                key={key}
                className="flex items-center justify-between gap-3 text-xs"
              >
                <span className="min-w-0 flex-1 truncate">{name}</span>
                <span
                  className={cn(
                    'shrink-0',
                    entry.suspended ? 'text-foreground-alt/60' : 'text-brand',
                  )}
                >
                  {entry.suspended ? 'Suspended' : 'Running'}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!!busyKey}
                  onClick={() => {
                    void handleToggle(entry)
                  }}
                  aria-label={`${entry.suspended ? 'Resume' : 'Suspend'} ${name}`}
                >
                  {busyKey === key ? (
                    <Spinner size="sm" />
                  ) : entry.suspended ? (
                    'Resume'
                  ) : (
                    'Suspend'
                  )}
                </Button>
              </div>
            )
          })}
        </div>
      ))}

      {actionError && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-2 py-1.5 text-xs">
          {actionError}
        </div>
      )}
    </div>
  )
}
