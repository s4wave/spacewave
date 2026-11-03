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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    minHeight: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontWeight: 'var(--font-weight-semibold)',
    userSelect: 'none',
    minWidth: 0,
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
    '--tw-tracking': 'var(--tracking-tight)',
    overflow: 'hidden',
    letterSpacing: 'var(--tracking-tight)',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule8: {
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    cursor: 'text',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule9: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule10: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    flexWrap: 'wrap',
    justifyContent: 'flex-end',
  },
  rule11: {
    display: { default: 'none', '@media (min-width: 48rem)': 'inline' },
  },
  rule12: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule13: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule14: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule15: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule17: {
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    userSelect: 'none',
  },
  rule18: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule19: { flex: 1, overflow: 'auto', minHeight: 0 },
  rule20: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule21: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: { flexShrink: 0 },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: { cursor: 'not-allowed', opacity: 0.5 },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
  renderOrgInfoSection?: (childSx: stylex.StyleXStyles) => ReactNode
  objectsBadge?: ReactNode
  objectsActions?: ReactNode
  objectsSection?: ReactNode
  settingsSection?: ReactNode
  renderDataSection?: (childSx: stylex.StyleXStyles) => ReactNode
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
  sx,
  canShare,
  onSharingClick,
  ...section
}: (SectionProps & { canShare: boolean; onSharingClick?: () => void }) & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  return (
    <CollapsibleSection
      sx={sx}
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
  sx,
  onExportClick,
  renderDataSection,
  ...section
}: (SectionProps & {
  onExportClick?: () => void
  renderDataSection?: (childSx: stylex.StyleXStyles) => ReactNode
}) & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <CollapsibleSection
      sx={sx}
      title="Data"
      icon={<LuDatabase sx={stylexStyles.rule1} />}
      {...section}
    >
      <div>
        <ActionCard
          sx={detailsChildren.child}
          icon={<LuDownload sx={stylexStyles.rule2} />}
          label="Export Data"
          description="Download object contents"
          onClick={onExportClick}
          compact={section.compact}
        />
        {renderDataSection?.(detailsChildren.child)}
      </div>
    </CollapsibleSection>
  )
}

/** SharedObjectIdentifiersSection renders the copyable object identifiers. */
function SharedObjectIdentifiersSection({
  sx,
  sharedObjectId,
  blockStoreId,
  peerId,
  renderOrgInfoSection,
  ...section
}: (SectionProps & {
  sharedObjectId: string
  blockStoreId: string
  peerId: string
  renderOrgInfoSection?: (childSx: stylex.StyleXStyles) => ReactNode
}) & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <CollapsibleSection
      sx={sx}
      title="Identifiers"
      icon={<LuCpu sx={stylexStyles.rule1} />}
      {...section}
    >
      <InfoCard compact={section.compact}>
        <div>
          <CopyableField
            sx={detailsChildren.child}
            label="Object ID"
            value={sharedObjectId}
          />
          <CopyableField
            sx={detailsChildren.child}
            label="Block Store"
            value={blockStoreId}
          />
          <CopyableField
            sx={detailsChildren.child}
            label="Peer ID"
            value={peerId}
          />
          {renderOrgInfoSection?.(detailsChildren.child)}
        </div>
      </InfoCard>
    </CollapsibleSection>
  )
}

/** SharedObjectDangerSection renders the delete action. */
function SharedObjectDangerSection({
  sx,
  onDeleteClick,
  ...section
}: (SectionProps & { onDeleteClick?: () => void }) & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const { compact } = section

  return (
    <CollapsibleSection sx={sx} title="Danger Zone" {...section}>
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
  renderOrgInfoSection,
  objectsBadge,
  objectsActions,
  objectsSection,
  settingsSection,
  renderDataSection,
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

  // Choose spacing once for every physical section.
  const sectionSpacing = compact ? nativeChildSpacing.s2 : nativeChildSpacing.s3

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
        <div sx={[compact ? false : false]}>
          {objectsSection && (
            <CollapsibleSection
              sx={sectionSpacing}
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
            sx={sectionSpacing}
            canShare={canShare}
            onSharingClick={onSharingClick}
            {...sectionProps('sharing')}
          />

          {settingsSection && (
            <CollapsibleSection
              sx={sectionSpacing}
              title="Settings"
              icon={<LuSettings sx={stylexStyles.rule1} />}
              {...sectionProps('settings')}
            >
              {settingsSection}
            </CollapsibleSection>
          )}

          <SharedObjectDataSection
            sx={sectionSpacing}
            onExportClick={onExportClick}
            renderDataSection={renderDataSection}
            {...sectionProps('data')}
          />

          {pluginsSection && (
            <CollapsibleSection
              sx={sectionSpacing}
              title="Plugins"
              icon={<LuPuzzle sx={stylexStyles.rule1} />}
              {...sectionProps('plugins')}
            >
              <InfoCard compact={compact}>{pluginsSection}</InfoCard>
            </CollapsibleSection>
          )}

          <SharedObjectIdentifiersSection
            sx={sectionSpacing}
            sharedObjectId={meta?.sharedObjectId ?? 'Unknown'}
            blockStoreId={meta?.blockStoreId ?? 'Unknown'}
            peerId={meta?.peerId ?? 'Unknown'}
            renderOrgInfoSection={renderOrgInfoSection}
            {...sectionProps('identifiers')}
          />

          <SharedObjectDangerSection
            sx={sectionSpacing}
            onDeleteClick={onDeleteClick}
            {...sectionProps('danger')}
          />
        </div>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})

// detailsChildren preserves spacing across data and identifier content slots.
const detailsChildren = stylex.create({
  child: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))',
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 2 * var(--tw-space-y-reverse))',
    },
  },
})
