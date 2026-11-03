import { LuBuilding2, LuPlus } from '@s4wave/web/images/icons.js'

import { SharedObjectDetails } from '@s4wave/app/sobject/SharedObjectDetails.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { collapsibleSectionActionStyles } from '@s4wave/web/ui/CollapsibleSection.js'

import { SpaceObjectBrowser } from './SpaceObjectBrowser.js'
import { SpacePlugins } from './SpacePlugins.js'
import { SpaceSettingsEditor } from './SpaceSettingsEditor.js'
import { SpaceDataSection } from './SpaceTransformSection.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-brand'],
    display: 'flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 20)`,
  },
  rule3: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

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
          <button type="button" onClick={onOrgClick} sx={stylexStyles2.rule1}>
            <LuBuilding2 sx={stylexStyles.rule1} />
            <span sx={stylexStyles2.rule2}>{org?.displayName || 'Org'}</span>
          </button>
        ) : undefined
      }
      renderOrgInfoSection={(childSx) =>
        org ? (
          <div sx={childSx}>
            <div sx={[nativeChildSpacing.s1, stylexStyles2.rule3]}>
              Organization
            </div>
            <div sx={[nativeChildSpacing.s1, stylexStyles2.rule4]}>
              <LuBuilding2 sx={stylexStyles.rule2} />
              <span sx={stylexStyles2.rule5}>{org.displayName || orgId}</span>
              <span sx={stylexStyles2.rule6}>
                {org.role === 'org:owner' ? 'Owner' : 'Member'}
              </span>
            </div>
          </div>
        ) : undefined
      }
      objectsBadge={
        <span sx={stylexStyles2.rule6}>
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
          <LuPlus sx={stylexStyles.rule3} />
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
      renderDataSection={(childSx) => <SpaceDataSection sx={childSx} />}
      pluginsSection={<SpacePlugins />}
    />
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})
