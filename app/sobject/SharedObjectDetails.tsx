import { useCallback, useMemo, type ReactNode } from 'react'
import {
  LuBox,
  LuCpu,
  LuDatabase,
  LuDownload,
  LuPencil,
  LuPuzzle,
  LuSettings,
  LuTrash2,
  LuUsers,
  LuUserPlus,
  LuX,
} from 'react-icons/lu'
import { PiAppStoreLogoBold } from 'react-icons/pi'

import { SpaceSoMeta } from '@s4wave/core/space/space.pb.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { ActionCard } from './ActionCard.js'
import { getBodyTypeName } from './body-type.js'
import { SpaceMembersPanel } from './SpaceMembersPanel.js'

export interface SharedObjectDetailsProps {
  displayName?: string
  canRename?: boolean
  canShare?: boolean
  onCloseClick?: () => void
  onSharingClick?: () => void
  onExportClick?: () => void
  onDeleteClick?: () => void
  onRenameStart?: () => void
  orgIndicator?: ReactNode
  orgInfoSection?: ReactNode
  objectsBadge?: ReactNode
  objectsActions?: ReactNode
  objectsSection?: ReactNode
  settingsSection?: ReactNode
  dataSection?: ReactNode
  pluginsSection?: ReactNode
}

type SharedObjectOpenSection =
  | 'objects'
  | 'sharing'
  | 'settings'
  | 'data'
  | 'plugins'
  | 'identifiers'
  | 'danger'
  | null

// SharedObjectDetails displays metadata and actions for a shared object.
export function SharedObjectDetails({
  displayName,
  canRename,
  canShare = true,
  onCloseClick,
  onSharingClick,
  onExportClick,
  onDeleteClick,
  onRenameStart,
  orgIndicator,
  orgInfoSection,
  objectsBadge,
  objectsActions,
  objectsSection,
  settingsSection,
  dataSection,
  pluginsSection,
}: SharedObjectDetailsProps) {
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const meta = sharedObject?.meta

  const sharedObjectId = meta?.sharedObjectId ?? 'Unknown'
  const blockStoreId = meta?.blockStoreId ?? 'Unknown'
  const peerId = meta?.peerId ?? 'Unknown'
  const bodyType = meta?.sharedObjectMeta?.bodyType ?? 'unknown'
  const bodyTypeName = getBodyTypeName(bodyType)
  const ns = useStateNamespace(['details'])
  const defaultOpenSection: SharedObjectOpenSection =
    objectsSection ? 'objects'
    : canShare ? 'sharing'
    : settingsSection ? 'settings'
    : 'data'
  const [openSection, setOpenSection] = useStateAtom<SharedObjectOpenSection>(
    ns,
    'open-section',
    defaultOpenSection,
  )
  const handleSectionOpenChange = useCallback(
    (section: Exclude<SharedObjectOpenSection, null>) => (open: boolean) => {
      setOpenSection(open ? section : null)
    },
    [setOpenSection],
  )

  const objectName = useMemo(() => {
    if (displayName) return displayName
    const bodyMeta = sharedObject?.meta?.sharedObjectMeta?.bodyMeta
    if (!bodyMeta || bodyMeta.length === 0) return 'Untitled'
    const spaceMeta = SpaceSoMeta.fromBinary(bodyMeta)
    return spaceMeta.name || 'Untitled'
  }, [displayName, sharedObject])

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
      <div className="border-foreground/8 flex min-h-9 shrink-0 items-center justify-between gap-3 border-b px-4 py-2">
        <div className="text-foreground flex min-w-0 flex-1 items-center gap-2 text-sm font-semibold select-none">
          <PiAppStoreLogoBold className="h-4 w-4" />
          <span
            className={cn(
              'min-w-0 truncate tracking-tight',
              canRename &&
                onRenameStart &&
                'hover:text-foreground-alt cursor-text transition-colors',
            )}
            onDoubleClick={
              canRename && onRenameStart ?
                (e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  onRenameStart()
                }
              : undefined
            }
          >
            {objectName}
          </span>
          <span className="text-foreground-alt/50 truncate">
            · {bodyTypeName}
          </span>
          {orgIndicator}
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
          {canRename && onRenameStart && (
            <Tooltip>
              <TooltipTrigger asChild>
                <DashboardButton
                  icon={<LuPencil className="h-3.5 w-3.5" />}
                  onClick={onRenameStart}
                >
                  <span className="hidden md:inline">Rename</span>
                </DashboardButton>
              </TooltipTrigger>
              <TooltipContent side="bottom">Rename space</TooltipContent>
            </Tooltip>
          )}
          {onCloseClick && (
            <Tooltip>
              <TooltipTrigger asChild>
                <DashboardButton
                  icon={<LuX className="h-4 w-4" />}
                  onClick={onCloseClick}
                />
              </TooltipTrigger>
              <TooltipContent side="bottom">Close</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        <div className="space-y-3">
          {objectsSection && (
            <CollapsibleSection
              title="Objects"
              icon={<LuBox className="h-3.5 w-3.5" />}
              open={openSection === 'objects'}
              onOpenChange={handleSectionOpenChange('objects')}
              badge={objectsBadge}
              headerActions={objectsActions}
            >
              {objectsSection}
            </CollapsibleSection>
          )}
          <CollapsibleSection
            title="Sharing"
            icon={<LuUsers className="h-3.5 w-3.5" />}
            open={openSection === 'sharing'}
            onOpenChange={handleSectionOpenChange('sharing')}
            headerActions={
              canShare && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      onClick={onSharingClick}
                      className="text-foreground-alt hover:text-foreground flex h-4 w-4 items-center justify-center transition-colors"
                      aria-label="Add user"
                      title="Add user"
                    >
                      <LuUserPlus className="h-3.5 w-3.5" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    Invite another person to this space
                  </TooltipContent>
                </Tooltip>
              )
            }
          >
            <SpaceMembersPanel />
          </CollapsibleSection>

          {settingsSection && (
            <CollapsibleSection
              title="Settings"
              icon={<LuSettings className="h-3.5 w-3.5" />}
              open={openSection === 'settings'}
              onOpenChange={handleSectionOpenChange('settings')}
            >
              {settingsSection}
            </CollapsibleSection>
          )}

          <CollapsibleSection
            title="Data"
            icon={<LuDatabase className="h-3.5 w-3.5" />}
            open={openSection === 'data'}
            onOpenChange={handleSectionOpenChange('data')}
          >
            <div className="space-y-2">
              <ActionCard
                icon={<LuDownload className="h-4 w-4" />}
                label="Export Data"
                description="Download object contents"
                onClick={onExportClick}
              />
              {dataSection}
            </div>
          </CollapsibleSection>

          {pluginsSection && (
            <CollapsibleSection
              title="Plugins"
              icon={<LuPuzzle className="h-3.5 w-3.5" />}
              open={openSection === 'plugins'}
              onOpenChange={handleSectionOpenChange('plugins')}
            >
              <InfoCard>{pluginsSection}</InfoCard>
            </CollapsibleSection>
          )}

          <CollapsibleSection
            title="Identifiers"
            icon={<LuCpu className="h-3.5 w-3.5" />}
            open={openSection === 'identifiers'}
            onOpenChange={handleSectionOpenChange('identifiers')}
          >
            <InfoCard>
              <div className="space-y-2">
                <CopyableField label="Object ID" value={sharedObjectId} />
                <CopyableField label="Block Store" value={blockStoreId} />
                <CopyableField label="Peer ID" value={peerId} />
                {orgInfoSection}
              </div>
            </InfoCard>
          </CollapsibleSection>

          <CollapsibleSection
            title="Danger Zone"
            open={openSection === 'danger'}
            onOpenChange={handleSectionOpenChange('danger')}
          >
            <button
              onClick={onDeleteClick}
              disabled={!onDeleteClick}
              className={cn(
                'border-destructive/30 bg-destructive/5 hover:border-destructive hover:bg-destructive/10 group flex w-full cursor-pointer items-center gap-3 rounded-lg border p-2.5 text-left transition-colors',
                !onDeleteClick && 'cursor-not-allowed opacity-50',
              )}
            >
              <div className="bg-destructive/20 group-hover:bg-destructive/30 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
                <LuTrash2 className="text-destructive h-3.5 w-3.5" />
              </div>
              <div className="flex min-w-0 flex-1 flex-col">
                <h4 className="text-destructive text-xs font-medium select-none">
                  Delete Object
                </h4>
                <p className="text-destructive/80 text-[0.6rem] select-none">
                  Permanently remove this object and all its data
                </p>
              </div>
            </button>
          </CollapsibleSection>
        </div>
      </div>
    </div>
  )
}
