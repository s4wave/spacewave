import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import { useCallback, useState } from 'react'
import { LuActivity } from '@s4wave/web/images/icons.js'

import type { BackgroundPlugin } from '@s4wave/core/session/session.pb.js'
import { knownSpacePlugin } from '@s4wave/app/space/known-plugins.js'
import { useWatchSpacesList } from '@s4wave/app/system/useSystemStatus.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'

import { Button } from '@s4wave/web/ui/button.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import {
  type BackgroundPlugins,
  useBackgroundPlugins,
} from './useBackgroundPlugins.js'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import * as stylex from '@stylexjs/stylex'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule3: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule6: { flexShrink: 0 },
  rule7: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule8: { color: themeColors['--color-brand'] },
  rule9: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: { color: themeColors['--color-brand'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// BackgroundSpace groups the background plugins of one Space.
interface BackgroundSpace {
  spaceId: string
  name: string
  plugins: BackgroundPlugin[]
}

// groupBySpace groups entries by Space in their saved order, naming each Space
// from names.
function groupBySpace(
  entries: BackgroundPlugin[],
  names: Map<string, string>,
): BackgroundSpace[] {
  const spaces: BackgroundSpace[] = []
  for (const entry of entries) {
    const spaceId = entry.spaceId ?? ''
    let space = spaces.find((space) => space.spaceId === spaceId)
    if (!space) {
      space = {
        spaceId,
        name: names.get(spaceId) || 'Untitled space',
        plugins: [],
      }
      spaces.push(space)
    }
    space.plugins.push(entry)
  }
  return spaces
}

// SessionBackgroundPluginsButton registers the bottom-bar item that lists the
// plugins confirmed to run while their Space is closed. It shows only while
// the Session has one.
export function SessionBackgroundPluginsButton() {
  const backgroundPlugins = useBackgroundPlugins()
  const entries = backgroundPlugins.entries
  const running = entries.filter((entry) => !entry.suspended).length
  const ariaLabel =
    running === 0
      ? 'Background plugins: all suspended'
      : `Background plugins: ${running} running`
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <Popover open={selected}>
        <PopoverAnchor asChild>
          <BottomBarItem
            selected={selected}
            onClick={onClick}
            sx={[sx, running !== 0 && stylexStyles2.rule1]}
            aria-label={ariaLabel}
            data-testid="session-background-plugins-button"
          >
            <LuActivity sx={stylexStyles.rule1} aria-hidden="true" />
          </BottomBarItem>
        </PopoverAnchor>
        <PopoverContent
          side="top"
          align="end"
          sideOffset={6}
          onEscapeKeyDown={onClick}
          onPointerDownOutside={onClick}
          variant="status"
        >
          <SessionBackgroundPluginsPopover
            backgroundPlugins={backgroundPlugins}
          />
        </PopoverContent>
      </Popover>
    ),
    [running, ariaLabel, backgroundPlugins],
  )

  if (entries.length === 0) return null

  return (
    <BottomBarLevel
      id="session-background-plugins"
      position="right"
      button={buttonRender}
    >
      {null}
    </BottomBarLevel>
  )
}

// SessionBackgroundPluginsPopover lists each Space with its background
// plugins and their Suspend and Resume controls.
function SessionBackgroundPluginsPopover({
  backgroundPlugins,
}: {
  backgroundPlugins: BackgroundPlugins
}) {
  const spacesList = useWatchSpacesList()
  const [busyKey, setBusyKey] = useState('')
  const [actionError, setActionError] = useState('')

  // Name each Space from the Session's Space list.
  const names = new Map<string, string>()
  for (const entry of spacesList ?? []) {
    const id = entry.entry?.ref?.providerResourceRef?.id
    if (id) names.set(id, entry.spaceMeta?.name ?? '')
  }
  const spaces = groupBySpace(backgroundPlugins.entries, names)

  const handleToggle = useCallback(
    async (entry: BackgroundPlugin) => {
      const key = `${entry.spaceId}/${entry.pluginId}`
      if (busyKey) return
      setBusyKey(key)
      setActionError('')
      try {
        await backgroundPlugins.set(
          entry.spaceId ?? '',
          entry.pluginId ?? '',
          true,
          !entry.suspended,
        )
      } catch (err) {
        setActionError(err instanceof Error ? err.message : String(err))
      } finally {
        setBusyKey('')
      }
    },
    [busyKey, backgroundPlugins],
  )

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3 p-3'
      }
      data-testid="session-background-plugins-popover"
    >
      <div>
        <div sx={stylexStyles3.rule1}>Background plugins</div>
        <div sx={stylexStyles3.rule2}>
          These keep running while their Space is closed.
        </div>
      </div>

      {spaces.map((space) => (
        <div key={space.spaceId} sx={stylexStyles4.rule1}>
          <div sx={[nativeChildSpacing.s1_5, stylexStyles3.rule3]}>
            {space.name}
          </div>
          {space.plugins.map((entry) => {
            const key = `${entry.spaceId}/${entry.pluginId}`
            const name = knownSpacePlugin(entry.pluginId ?? '').name
            return (
              <div
                key={key}
                sx={[nativeChildSpacing.s1_5, stylexStyles3.rule4]}
              >
                <span sx={stylexStyles3.rule5}>{name}</span>
                <span
                  sx={[
                    stylexStyles3.rule6,
                    entry.suspended ? stylexStyles3.rule7 : stylexStyles3.rule8,
                  ]}
                >
                  {entry.suspended ? 'Suspended' : 'Running'}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!!busyKey}
                  onClick={() => {
                    void handleToggle(entry)
                  }}
                  aria-label={`${entry.suspended ? 'Resume' : 'Suspend'} ${name}`}
                >
                  {busyKey === key ? (
                    <Spinner size="sm" />
                  ) : entry.suspended ? (
                    'Resume'
                  ) : (
                    'Suspend'
                  )}
                </Button>
              </div>
            )
          })}
        </div>
      ))}

      {actionError && <div sx={stylexStyles3.rule9}>{actionError}</div>}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
