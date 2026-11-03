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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule2: {
    minHeight: `calc(${themeLayout['--spacing']} * 8)`,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule3: {
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule8: {
    cursor: 'text',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
  },
  rule9: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
    display: 'flex',
    flexShrink: 0,
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'flex-end',
    gap: themeLayout['--spacing'],
  },
  rule11: {
    display: { default: 'none', '@media (min-width: 48rem)': 'inline' },
  },
  rule12: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule13: {
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule14: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule15: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule17: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule18: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule19: { minHeight: 0, flex: 1, overflow: 'auto' },
  rule20: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule21: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
      sx={[
        stylexStyles4.rule1,
        compact ? stylexStyles4.rule2 : stylexStyles4.rule3,
      ]}
    >
      <div
        sx={[
          stylexStyles4.rule4,
          compact ? stylexStyles4.rule5 : stylexStyles4.rule6,
        ]}
      >
        <PiAppStoreLogoBold
          sx={[
            stylexStyles3.rule1,
            compact ? stylexStyles3.rule2 : stylexStyles3.rule3,
          ]}
        />
        <span
          sx={[stylexStyles4.rule7, !!onRenameStart && stylexStyles4.rule8]}
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
        {!compact && <span sx={stylexStyles4.rule9}>· {bodyTypeName}</span>}
        {orgIndicator}
      </div>
      <div sx={stylexStyles4.rule10}>
        {onRenameStart && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuPencil sx={stylexStyles.rule1} />}
                onClick={onRenameStart}
              >
                <span sx={stylexStyles4.rule11}>Rename</span>
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
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
        }
      >
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
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
          }
        >
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
          sx={[
            stylexStyles4.rule12,
            compact ? stylexStyles4.rule13 : stylexStyles4.rule14,
          ]}
        >
          <LuTrash2 sx={stylexStyles.rule3} />
        </div>
        <div sx={stylexStyles4.rule15}>
          <h4 sx={stylexStyles4.rule16}>Delete Object</h4>
          {!compact && (
            <p sx={stylexStyles4.rule17}>
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
    <div ref={containerRef} sx={stylexStyles4.rule18}>
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
        sx={[
          stylexStyles4.rule19,
          compact ? stylexStyles4.rule20 : stylexStyles4.rule21,
        ]}
      >
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ cn(
              compact ? 'space-y-2' : 'space-y-3',
            )
          }
        >
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
