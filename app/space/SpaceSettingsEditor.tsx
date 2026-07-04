import { useMemo, useCallback } from 'react'
import { LuKeyboard, LuPencil, LuSettings } from 'react-icons/lu'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { ObjectKeySelector } from '@s4wave/web/ui/ObjectKeySelector.js'

import { useInvokeCommand } from '@s4wave/web/command/index.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { buildObjectTree } from '@s4wave/web/space/object-tree.js'
import { applySpaceIndexPath } from './space-settings.js'

interface SpaceSettingsEditorProps {
  canEdit: boolean
  canRename: boolean
  displayName: string
  embedded?: boolean
  onRenameStart?: () => void
}

// SpaceSettingsEditor renders the settings section with an ObjectKeySelector for index_path.
export function SpaceSettingsEditor({
  canEdit,
  canRename,
  displayName,
  embedded,
  onRenameStart,
}: SpaceSettingsEditorProps) {
  const { spaceState, spaceWorld } = SpaceContainerContext.useContext()
  const invokeCommand = useInvokeCommand()

  const indexPath = spaceState.settings?.indexPath ?? ''
  const worldObjects = spaceState.worldContents?.objects
  const treeNodes = useMemo(
    () => buildObjectTree(worldObjects ?? []),
    [worldObjects],
  )

  const handleIndexPathChange = useCallback(
    async (newPath: string) => {
      if (!spaceWorld || newPath === indexPath) return
      await applySpaceIndexPath(spaceWorld, spaceState.settings, newPath)
    },
    [spaceWorld, spaceState.settings, indexPath],
  )

  const content = (
    <>
      {!embedded && (
        <div className="mb-2 flex items-center justify-between">
          <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuSettings className="size-3.5" />
            Settings
          </h2>
        </div>
      )}
      <InfoCard>
        <div className="space-y-2">
          <div>
            <span className="text-foreground-alt mb-1 block text-[0.6rem] select-none">
              Display Name
            </span>
            {canRename && onRenameStart ? (
              <div className="flex items-center justify-between gap-2">
                <div
                  className="text-foreground hover:text-foreground-alt min-w-0 flex-1 cursor-text text-xs transition-colors"
                  role="button"
                  tabIndex={0}
                  onDoubleClick={() => onRenameStart()}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      onRenameStart()
                    }
                  }}
                >
                  {displayName || 'Untitled'}
                </div>
                <DashboardButton
                  icon={<LuPencil className="size-3" />}
                  onClick={() => onRenameStart()}
                >
                  Rename
                </DashboardButton>
              </div>
            ) : (
              <div className="text-foreground text-xs">
                {displayName || 'Untitled'}
              </div>
            )}
          </div>
          <div>
            <span className="text-foreground-alt mb-1 block text-[0.6rem] select-none">
              Index Path
            </span>
            {canEdit ? (
              <ObjectKeySelector
                nodes={treeNodes}
                value={indexPath}
                onChange={(newPath) => void handleIndexPathChange(newPath)}
                placeholder="No default view"
              />
            ) : (
              <div className="text-foreground text-sm">
                {indexPath || 'Not set'}
              </div>
            )}
          </div>
        </div>
      </InfoCard>
      <button
        type="button"
        className="border-foreground/10 bg-foreground/5 hover:border-brand/30 hover:bg-brand/5 group mt-2 flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors"
        onClick={() =>
          invokeCommand('spacewave.preferences.keyboard-shortcuts', {
            scope: 'space',
          })
        }
      >
        <div className="bg-foreground/10 group-hover:bg-brand/10 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors">
          <LuKeyboard className="text-foreground-alt group-hover:text-brand size-3.5 transition-colors" />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <h4 className="text-foreground text-xs font-medium select-none">
            Space Overrides
          </h4>
          <p className="text-foreground-alt text-xs select-none">
            Open keyboard shortcut overrides for this Space
          </p>
        </div>
      </button>
    </>
  )

  if (embedded) return content

  return <section>{content}</section>
}
