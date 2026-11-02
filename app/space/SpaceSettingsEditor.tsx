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

const stylexStyles2 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
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
        <div className="mb-2 flex items-center justify-between">
          <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuSettings sx={stylexStyles.rule1} />
            Settings
          </h2>
        </div>
      )}
      <InfoCard>
        <div className="space-y-2">
          <div>
            <span className="text-foreground-alt mb-1 block text-xs select-none">
              Display Name
            </span>
            {canRename && onRenameStart ? (
              <div className="flex items-center justify-between gap-2">
                <button
                  type="button"
                  className="text-foreground hover:text-foreground-alt min-w-0 flex-1 cursor-text text-left text-xs transition-colors"
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
              <div className="text-foreground text-xs">
                {displayName || 'Untitled'}
              </div>
            )}
          </div>
          <div>
            <span className="text-foreground-alt mb-1 block text-xs select-none">
              Index Path
            </span>
            {canEdit ? (
              <ObjectKeySelector
                world={spaceWorldResource}
                value={shownIndexPath}
                onChange={(newPath) => void handleIndexPathChange(newPath)}
                placeholder="No default view"
              />
            ) : (
              <div className="text-foreground text-sm">
                {indexPath || 'Not set'}
              </div>
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
        <div className="bg-foreground/10 group-hover:bg-brand/10 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors">
          <LuKeyboard sx={stylexStyles.rule3} />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <h4 className="text-foreground text-xs font-medium select-none">
            Space Overrides
          </h4>
          <p className="text-foreground-alt text-xs select-none">
            Open keyboard shortcut overrides for this Space
          </p>
        </div>
      </button>
    </>
  )

  if (embedded) return content

  return <section>{content}</section>
}
