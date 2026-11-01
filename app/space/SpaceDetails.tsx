import { LuBuilding2, LuPlus } from '@s4wave/web/images/icons.js'

import { SharedObjectDetails } from '@s4wave/app/sobject/SharedObjectDetails.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { collapsibleSectionActionStyles } from '@s4wave/web/ui/CollapsibleSection.js'

import { SpaceObjectBrowser } from './SpaceObjectBrowser.js'
import { SpacePlugins } from './SpacePlugins.js'
import { SpaceSettingsEditor } from './SpaceSettingsEditor.js'
import { SpaceDataSection } from './SpaceTransformSection.js'

// SpaceDetailsProps are the Space actions the details panel offers.
export interface SpaceDetailsProps {
  spaceName: string
  canRename: boolean
  canShare: boolean
  orgId?: string
  onClose: () => void
  onSharing: () => void
  onExport: () => void
  onDelete: () => void
  onRename: () => void
  onCreateObject: () => void
  onOrgClick: () => void
}

// SpaceDetails is the Space's bottom bar details panel. It renders inside the
// Space's bottom bar level, so it reads the live Space state from context.
export function SpaceDetails({
  spaceName,
  canRename,
  canShare,
  orgId,
  onClose,
  onSharing,
  onExport,
  onDelete,
  onRename,
  onCreateObject,
  onOrgClick,
}: SpaceDetailsProps) {
  const { orgState, spaceObjectTargets } = SpaceContainerContext.useContext()
  const org = orgId ? orgState?.organization : undefined

  return (
    <SharedObjectDetails
      displayName={spaceName}
      canRename={canRename}
      canShare={canShare}
      onCloseClick={onClose}
      onSharingClick={canShare ? onSharing : undefined}
      onExportClick={onExport}
      onDeleteClick={onDelete}
      onRenameStart={onRename}
      orgIndicator={
        orgId ? (
          <button
            type="button"
            onClick={onOrgClick}
            className="bg-brand/10 text-brand hover:bg-brand/20 flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs font-medium transition-colors"
          >
            <LuBuilding2 className="size-2.5" />
            <span className="max-w-20 truncate">
              {org?.displayName || 'Org'}
            </span>
          </button>
        ) : undefined
      }
      orgInfoSection={
        org ? (
          <div className="space-y-1">
            <div className="text-foreground-alt mb-0.5 text-xs select-none">
              Organization
            </div>
            <div className="text-foreground flex items-center gap-1.5 text-xs">
              <LuBuilding2 className="text-brand size-3 shrink-0" />
              <span className="truncate">{org.displayName || orgId}</span>
              <span className="text-foreground-alt/50 text-xs">
                {org.role === 'org:owner' ? 'Owner' : 'Member'}
              </span>
            </div>
          </div>
        ) : undefined
      }
      objectsBadge={
        <span className="text-foreground-alt/50 text-xs">
          {spaceObjectTargets?.targets.length ?? 0}
          {spaceObjectTargets?.more ? '+' : ''}
        </span>
      }
      objectsActions={
        <button
          type="button"
          onClick={onCreateObject}

          sx={collapsibleSectionActionStyles.button}
          aria-label="Create object"
          title="Create object"
        >
          <LuPlus className="size-3.5" />
        </button>
      }
      objectsSection={<SpaceObjectBrowser embedded={true} />}
      settingsSection={
        <SpaceSettingsEditor
          canEdit={true}
          canRename={canRename}
          displayName={spaceName}
          embedded={true}
          onRenameStart={onRename}
        />
      }
      dataSection={<SpaceDataSection />}
      pluginsSection={<SpacePlugins />}
    />
  )
}
