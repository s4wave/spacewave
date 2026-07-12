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
import { useContainerDensity } from '@s4wave/web/hooks/useContainerDensity.js'
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
  const { ref: containerRef, density } = useContainerDensity()
  const compact = density === 'compact'
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const meta = sharedObject?.meta

  const sharedObjectId = meta?.sharedObjectId ?? 'Unknown'
  const blockStoreId = meta?.blockStoreId ?? 'Unknown'
  const peerId = meta?.peerId ?? 'Unknown'
  const bodyType = meta?.sharedObjectMeta?.bodyType ?? 'unknown'
  const bodyTypeName = getBodyTypeName(bodyType)
  const ns = useStateNamespace(['details'])
  const defaultOpenSection: SharedObjectOpenSection = objectsSection
    ? 'objects'
    : canShare
      ? 'sharing'
      : settingsSection
        ? 'settings'
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
    <div
      ref={containerRef}
      className="bg-background-primary flex h-full w-full flex-col overflow-hidden"
    >
      <div
        className={cn(
          'border-foreground/8 flex shrink-0 items-center justify-between border-b',
          compact ? 'min-h-8 gap-1.5 px-2.5 py-1.5' : 'min-h-9 gap-3 px-4 py-2',
        )}
      >
        <div
          className={cn(
            'text-foreground flex min-w-0 flex-1 items-center gap-2 font-semibold select-none',
            compact ? 'text-xs' : 'text-sm',
          )}
        >
          <PiAppStoreLogoBold
            className={cn('shrink-0', compact ? 'size-3.5' : 'size-4')}
          />
          <span
            className={cn(
              'min-w-0 truncate tracking-tight',
              canRename &&
                onRenameStart &&
                'hover:text-foreground-alt cursor-text transition-colors',
            )}
            onDoubleClick={
              canRename && onRenameStart
                ? (e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    onRenameStart()
                  }
                : undefined
            }
          >
            {objectName}
          </span>
          {!compact && (
            <span className="text-foreground-alt/50 truncate">
              · {bodyTypeName}
            </span>
          )}
          {orgIndicator}
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
          {canRename && onRenameStart && (
            <Tooltip>
              <TooltipTrigger asChild>
                <DashboardButton
                  icon={<LuPencil className="size-3.5" />}
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
                  icon={<LuX className="size-4" />}
                  onClick={onCloseClick}
                />
              </TooltipTrigger>
              <TooltipContent side="bottom">Close</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>

      <div
        className={cn(
          'min-h-0 flex-1 overflow-auto',
          compact ? 'px-2.5 py-2' : 'px-4 py-3',
        )}
      >
        <div className={cn(compact ? 'space-y-2' : 'space-y-3')}>
          {objectsSection && (
            <CollapsibleSection
              title="Objects"
              icon={<LuBox className="size-3.5" />}
              open={openSection === 'objects'}
              onOpenChange={handleSectionOpenChange('objects')}
              badge={objectsBadge}
              headerActions={objectsActions}
              compact={compact}
            >
              {objectsSection}
            </CollapsibleSection>
          )}
          <CollapsibleSection
            title="Sharing"
            icon={<LuUsers className="size-3.5" />}
            open={openSection === 'sharing'}
            onOpenChange={handleSectionOpenChange('sharing')}
            compact={compact}
            headerActions={
              canShare && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      onClick={onSharingClick}
                      className="text-foreground-alt hover:text-foreground flex size-4 items-center justify-center transition-colors"
                      aria-label="Add user"
                      title="Add user"
                    >
                      <LuUserPlus className="size-3.5" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    Invite another person to this space
                  </TooltipContent>
                </Tooltip>
              )
            }
          >
            <SpaceMembersPanel compact={compact} />
          </CollapsibleSection>

          {settingsSection && (
            <CollapsibleSection
              title="Settings"
              icon={<LuSettings className="size-3.5" />}
              open={openSection === 'settings'}
              onOpenChange={handleSectionOpenChange('settings')}
              compact={compact}
            >
              {settingsSection}
            </CollapsibleSection>
          )}

          <CollapsibleSection
            title="Data"
            icon={<LuDatabase className="size-3.5" />}
            open={openSection === 'data'}
            onOpenChange={handleSectionOpenChange('data')}
            compact={compact}
          >
            <div className="space-y-2">
              <ActionCard
                icon={<LuDownload className="size-4" />}
                label="Export Data"
                description="Download object contents"
                onClick={onExportClick}
                compact={compact}
              />
              {dataSection}
            </div>
          </CollapsibleSection>

          {pluginsSection && (
            <CollapsibleSection
              title="Plugins"
              icon={<LuPuzzle className="size-3.5" />}
              open={openSection === 'plugins'}
              onOpenChange={handleSectionOpenChange('plugins')}
              compact={compact}
            >
              <InfoCard compact={compact}>{pluginsSection}</InfoCard>
            </CollapsibleSection>
          )}

          <CollapsibleSection
            title="Identifiers"
            icon={<LuCpu className="size-3.5" />}
            open={openSection === 'identifiers'}
            onOpenChange={handleSectionOpenChange('identifiers')}
            compact={compact}
          >
            <InfoCard compact={compact}>
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
            compact={compact}
          >
            <button
              onClick={onDeleteClick}
              disabled={!onDeleteClick}
              className={cn(
                'border-destructive/30 bg-destructive/5 hover:border-destructive hover:bg-destructive/10 group flex w-full cursor-pointer items-center rounded-lg border text-left transition-colors',
                compact ? 'gap-2 p-2' : 'gap-3 p-2.5',
                !onDeleteClick && 'cursor-not-allowed opacity-50',
              )}
            >
              <div
                className={cn(
                  'bg-destructive/20 group-hover:bg-destructive/30 flex shrink-0 items-center justify-center rounded-md transition-colors',
                  compact ? 'size-7' : 'size-8',
                )}
              >
                <LuTrash2 className="text-destructive size-3.5" />
              </div>
              <div className="flex min-w-0 flex-1 flex-col">
                <h4 className="text-destructive text-xs font-medium select-none">
                  Delete Object
                </h4>
                {!compact && (
                  <p className="text-destructive/80 text-[0.6rem] select-none">
                    Permanently remove this object and all its data
                  </p>
                )}
              </div>
            </button>
          </CollapsibleSection>
        </div>
      </div>
    </div>
  )
}
