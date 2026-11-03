import { useCallback, useState } from 'react'
import { LuKeyboard, LuPencil, LuSettings } from '@s4wave/web/images/icons.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { ObjectKeySelector } from '@s4wave/web/ui/ObjectKeySelector.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { useInvokeCommand } from '@s4wave/web/command/index.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { applySpaceIndexPath } from './space-settings.js'
import { SpaceBackfillSection } from './SpaceBackfillSection.js'
import { SpaceControlSection } from './SpaceControlSection.js'
import { SpaceSyncSection } from './SpaceSyncSection.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule3: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: themeLayout['--spacing'],
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule5: {
    flex: 1,
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    cursor: 'text',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minWidth: 0,
  },
  rule6: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
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
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule9: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule11: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
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
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface SpaceSettingsEditorProps {
  canEdit: boolean
  canRename: boolean
  displayName: string
  embedded?: boolean
  onRenameStart?: () => void
}

// PendingIndexPath is an index path write the watched settings do not show yet.
interface PendingIndexPath {
  from: string
  to: string
}

// SpaceSettingsEditor renders the settings section with an ObjectKeySelector for index_path.
export function SpaceSettingsEditor({
  canEdit,
  canRename,
  displayName,
  embedded,
  onRenameStart,
}: SpaceSettingsEditorProps) {
  const { spaceState, spaceWorld, spaceWorldResource } =
    SpaceContainerContext.useContext()
  const invokeCommand = useInvokeCommand()

  // The selector shows a chosen path from the moment it is picked. The choice
  // stays pending until the watched settings move off the path it replaced,
  // or the write fails and the selector falls back to the watched path.
  const indexPath = spaceState.settings?.indexPath ?? ''
  const [pending, setPending] = useState<PendingIndexPath | null>(null)
  const settled =
    !pending || pending.from !== indexPath || pending.to === indexPath
  if (pending && settled) setPending(null)
  const shownIndexPath = settled ? indexPath : pending.to

  const handleIndexPathChange = useCallback(
    async (newPath: string) => {
      if (!spaceWorld || newPath === shownIndexPath) return
      const choice = { from: indexPath, to: newPath }
      setPending(choice)
      try {
        await applySpaceIndexPath(spaceWorld, newPath)
      } catch (err) {
        setPending((cur) => (cur === choice ? null : cur))
        toast.error('Could not change the index path', {
          description: err instanceof Error ? err.message : String(err),
        })
      }
    },
    [spaceWorld, indexPath, shownIndexPath],
  )

  const content = (
    <>
      {!embedded && (
        <div sx={stylexStyles3.rule1}>
          <h2 sx={stylexStyles3.rule2}>
            <LuSettings sx={stylexStyles.rule1} />
            Settings
          </h2>
        </div>
      )}
      <InfoCard>
        <div>
          <div sx={nativeChildSpacing.s2}>
            <span sx={stylexStyles3.rule3}>Display Name</span>
            {canRename && onRenameStart ? (
              <div sx={stylexStyles3.rule4}>
                <button
                  type="button"
                  sx={stylexStyles3.rule5}
                  onDoubleClick={() => onRenameStart()}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      onRenameStart()
                    }
                  }}
                >
                  {displayName || 'Untitled'}
                </button>
                <DashboardButton
                  icon={<LuPencil sx={stylexStyles.rule2} />}
                  onClick={() => onRenameStart()}
                >
                  Rename
                </DashboardButton>
              </div>
            ) : (
              <div sx={stylexStyles3.rule6}>{displayName || 'Untitled'}</div>
            )}
          </div>
          <div sx={nativeChildSpacing.s2}>
            <span sx={stylexStyles3.rule3}>Index Path</span>
            {canEdit ? (
              <ObjectKeySelector
                world={spaceWorldResource}
                value={shownIndexPath}
                onChange={(newPath) => void handleIndexPathChange(newPath)}
                placeholder="No default view"
              />
            ) : (
              <div sx={stylexStyles3.rule7}>{indexPath || 'Not set'}</div>
            )}
          </div>
        </div>
      </InfoCard>
      <SpaceSyncSection />
      <SpaceControlSection />
      <SpaceBackfillSection />
      <button
        type="button"
        sx={[stylexStyles2.rule1, groupMarker2]}
        onClick={() =>
          invokeCommand('spacewave.preferences.keyboard-shortcuts', {
            scope: 'space',
          })
        }
      >
        <div sx={stylexStyles3.rule8}>
          <LuKeyboard sx={stylexStyles.rule3} />
        </div>
        <div sx={stylexStyles3.rule9}>
          <h4 sx={stylexStyles3.rule10}>Space Overrides</h4>
          <p sx={stylexStyles3.rule11}>
            Open keyboard shortcut overrides for this Space
          </p>
        </div>
      </button>
    </>
  )

  if (embedded) return content

  return <section>{content}</section>
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
})
