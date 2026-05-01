import { useMemo, useCallback } from 'react'
import { LuPencil, LuSettings } from 'react-icons/lu'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { ObjectKeySelector } from '@s4wave/web/ui/ObjectKeySelector.js'

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
            <LuSettings className="h-3.5 w-3.5" />
            Settings
          </h2>
        </div>
      )}
      <InfoCard>
        <div className="space-y-2">
          <div>
            <label className="text-foreground-alt mb-1 block text-[0.6rem] select-none">
              Display Name
            </label>
            {canRename && onRenameStart ?
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
                  icon={<LuPencil className="h-3 w-3" />}
                  onClick={() => onRenameStart()}
                >
                  Rename
                </DashboardButton>
              </div>
            : <div className="text-foreground text-xs">
                {displayName || 'Untitled'}
              </div>
            }
          </div>
          <div>
            <label className="text-foreground-alt mb-1 block text-[0.6rem] select-none">
              Index Path
            </label>
            {canEdit ?
              <ObjectKeySelector
                nodes={treeNodes}
                value={indexPath}
                onChange={(newPath) => void handleIndexPathChange(newPath)}
                placeholder="No default view"
              />
            : <div className="text-foreground text-sm">
                {indexPath || 'Not set'}
              </div>
            }
          </div>
        </div>
      </InfoCard>
    </>
  )

  if (embedded) return content

  return <section>{content}</section>
}
