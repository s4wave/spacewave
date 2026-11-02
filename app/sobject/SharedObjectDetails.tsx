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
} from '@s4wave/web/images/icons.js'
import { PiAppStoreLogoBold } from '@s4wave/web/ui/phosphor-icons.js'

import { SpaceSoMeta } from '@s4wave/core/space/space.pb.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useContainerDensity } from '@s4wave/web/hooks/useContainerDensity.js'
import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  CollapsibleSection,
  collapsibleSectionActionStyles,
} from '@s4wave/web/ui/CollapsibleSection.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { ActionCard } from './ActionCard.js'
import { getBodyTypeName } from './body-type.js'
import { SpaceMembersPanel } from './SpaceMembersPanel.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { flexShrink: 0 },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule4: { cursor: 'not-allowed', opacity: 0.5 },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-destructive'],
  },
})

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
          sx={[
            stylexStyles3.rule1,
            compact ? stylexStyles3.rule2 : stylexStyles3.rule3,
          ]}
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
                icon={<LuPencil sx={stylexStyles.rule1} />}
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
                icon={<LuX sx={stylexStyles.rule2} />}
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
      icon={<LuUsers sx={stylexStyles.rule1} />}
      {...section}
      headerActions={
        canShare && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                onClick={onSharingClick}

                sx={collapsibleSectionActionStyles.button}
                aria-label="Add user"
                title="Add user"
              >
                <LuUserPlus sx={stylexStyles.rule1} />
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
      icon={<LuDatabase sx={stylexStyles.rule1} />}
      {...section}
    >
      <div className="space-y-2">
        <ActionCard
          icon={<LuDownload sx={stylexStyles.rule2} />}
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
      icon={<LuCpu sx={stylexStyles.rule1} />}
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
        sx={[
          [stylexStyles2.rule1, groupMarker],
          compact ? stylexStyles2.rule2 : stylexStyles2.rule3,
          !onDeleteClick && stylexStyles2.rule4,
        ]}
      >
        <div
          className={cn(
            'bg-destructive/20 group-hover:bg-destructive/30 flex shrink-0 items-center justify-center rounded-md transition-colors',
            compact ? 'size-7' : 'size-8',
          )}
        >
          <LuTrash2 sx={stylexStyles.rule3} />
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
  // Read the panel density and the live object's metadata.
  const { ref: containerRef, density } = useContainerDensity()
  const compact = density === 'compact'
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const meta = sharedObject?.meta

  // Keep the current details section in its saved state namespace.
  const ns = useStateNamespace(['details'])
  const [openSection, setOpenSection] = useStateAtom<SharedObjectOpenSection>(
    ns,
    'open-section',
    defaultSectionFor(!!objectsSection, canShare, !!settingsSection),
  )

  // Give every section one controlled expansion state and the current density.
  const sectionProps = useCallback(
    (section: Exclude<SharedObjectOpenSection, null>): SectionProps => ({
      open: openSection === section,
      onOpenChange: (open) => setOpenSection(open ? section : null),
      compact,
    }),
    [openSection, setOpenSection, compact],
  )

  // Resolve the visible object name from its current metadata.
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
              icon={<LuBox sx={stylexStyles.rule1} />}
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
              icon={<LuSettings sx={stylexStyles.rule1} />}
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
              icon={<LuPuzzle sx={stylexStyles.rule1} />}
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
