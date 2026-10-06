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

interface SectionProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  compact: boolean
}

/** defaultSectionFor picks the section that is open before the user chooses. */
function defaultSectionFor(
  hasObjects: boolean,
  canShare: boolean,
  hasSettings: boolean,
): SharedObjectOpenSection {
  if (hasObjects) return 'objects'
  if (canShare) return 'sharing'
  return hasSettings ? 'settings' : 'data'
}

/** resolveObjectName returns the display name, falling back to the Space meta. */
function resolveObjectName(
  displayName: string | undefined,
  bodyMeta: Uint8Array | undefined,
): string {
  if (displayName) return displayName
  if (!bodyMeta || bodyMeta.length === 0) return 'Untitled'
  return SpaceSoMeta.fromBinary(bodyMeta).name || 'Untitled'
}

/** SharedObjectHeader renders the object title with rename and close actions. */
function SharedObjectHeader({
  objectName,
  bodyTypeName,
  compact,
  onRenameStart,
  onCloseClick,
  orgIndicator,
}: {
  objectName: string
  bodyTypeName: string
  compact: boolean
  onRenameStart?: () => void
  onCloseClick?: () => void
  orgIndicator?: ReactNode
}) {
  return (
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
            onRenameStart &&
              'hover:text-foreground-alt cursor-text transition-colors',
          )}
          onDoubleClick={
            onRenameStart
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
        {onRenameStart && (
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
  )
}

/** SharedObjectSharingSection renders the member list and the add user action. */
function SharedObjectSharingSection({
  canShare,
  onSharingClick,
  ...section
}: SectionProps & { canShare: boolean; onSharingClick?: () => void }) {
  return (
    <CollapsibleSection
      title="Sharing"
      icon={<LuUsers className="size-3.5" />}
      {...section}
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
      <SpaceMembersPanel compact={section.compact} />
    </CollapsibleSection>
  )
}

/** SharedObjectDataSection renders the export action and extra data controls. */
function SharedObjectDataSection({
  onExportClick,
  dataSection,
  ...section
}: SectionProps & { onExportClick?: () => void; dataSection?: ReactNode }) {
  return (
    <CollapsibleSection
      title="Data"
      icon={<LuDatabase className="size-3.5" />}
      {...section}
    >
      <div className="space-y-2">
        <ActionCard
          icon={<LuDownload className="size-4" />}
          label="Export Data"
          description="Download object contents"
          onClick={onExportClick}
          compact={section.compact}
        />
        {dataSection}
      </div>
    </CollapsibleSection>
  )
}

/** SharedObjectIdentifiersSection renders the copyable object identifiers. */
function SharedObjectIdentifiersSection({
  sharedObjectId,
  blockStoreId,
  peerId,
  orgInfoSection,
  ...section
}: SectionProps & {
  sharedObjectId: string
  blockStoreId: string
  peerId: string
  orgInfoSection?: ReactNode
}) {
  return (
    <CollapsibleSection
      title="Identifiers"
      icon={<LuCpu className="size-3.5" />}
      {...section}
    >
      <InfoCard compact={section.compact}>
        <div className="space-y-2">
          <CopyableField label="Object ID" value={sharedObjectId} />
          <CopyableField label="Block Store" value={blockStoreId} />
          <CopyableField label="Peer ID" value={peerId} />
          {orgInfoSection}
        </div>
      </InfoCard>
    </CollapsibleSection>
  )
}

/** SharedObjectDangerSection renders the delete action. */
function SharedObjectDangerSection({
  onDeleteClick,
  ...section
}: SectionProps & { onDeleteClick?: () => void }) {
  const { compact } = section

  return (
    <CollapsibleSection title="Danger Zone" {...section}>
      <button
        type="button"
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
            <p className="text-destructive/80 micro-text select-none">
              Permanently remove this object and all its data
            </p>
          )}
        </div>
      </button>
    </CollapsibleSection>
  )
}

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
  const ns = useStateNamespace(['details'])
  const [openSection, setOpenSection] = useStateAtom<SharedObjectOpenSection>(
    ns,
    'open-section',
    defaultSectionFor(!!objectsSection, canShare, !!settingsSection),
  )

  const sectionProps = useCallback(
    (section: Exclude<SharedObjectOpenSection, null>): SectionProps => ({
      open: openSection === section,
      onOpenChange: (open) => setOpenSection(open ? section : null),
      compact,
    }),
    [openSection, setOpenSection, compact],
  )

  const bodyMeta = meta?.sharedObjectMeta?.bodyMeta
  const objectName = useMemo(
    () => resolveObjectName(displayName, bodyMeta),
    [displayName, bodyMeta],
  )

  return (
    <div
      ref={containerRef}
      className="bg-background-primary flex h-full w-full flex-col overflow-hidden"
    >
      <SharedObjectHeader
        objectName={objectName}
        bodyTypeName={getBodyTypeName(
          meta?.sharedObjectMeta?.bodyType ?? 'unknown',
        )}
        compact={compact}
        onRenameStart={canRename ? onRenameStart : undefined}
        onCloseClick={onCloseClick}
        orgIndicator={orgIndicator}
      />

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
              {...sectionProps('objects')}
              badge={objectsBadge}
              headerActions={objectsActions}
            >
              {objectsSection}
            </CollapsibleSection>
          )}
          <SharedObjectSharingSection
            canShare={canShare}
            onSharingClick={onSharingClick}
            {...sectionProps('sharing')}
          />

          {settingsSection && (
            <CollapsibleSection
              title="Settings"
              icon={<LuSettings className="size-3.5" />}
              {...sectionProps('settings')}
            >
              {settingsSection}
            </CollapsibleSection>
          )}

          <SharedObjectDataSection
            onExportClick={onExportClick}
            dataSection={dataSection}
            {...sectionProps('data')}
          />

          {pluginsSection && (
            <CollapsibleSection
              title="Plugins"
              icon={<LuPuzzle className="size-3.5" />}
              {...sectionProps('plugins')}
            >
              <InfoCard compact={compact}>{pluginsSection}</InfoCard>
            </CollapsibleSection>
          )}

          <SharedObjectIdentifiersSection
            sharedObjectId={meta?.sharedObjectId ?? 'Unknown'}
            blockStoreId={meta?.blockStoreId ?? 'Unknown'}
            peerId={meta?.peerId ?? 'Unknown'}
            orgInfoSection={orgInfoSection}
            {...sectionProps('identifiers')}
          />

          <SharedObjectDangerSection
            onDeleteClick={onDeleteClick}
            {...sectionProps('danger')}
          />
        </div>
      </div>
    </div>
  )
}
