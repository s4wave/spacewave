import type { IconType } from '@s4wave/web/images/icons.js'
import { type ReactNode, useCallback, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import {
  LuLoaderCircle,
  LuPlus,
  LuPuzzle,
  LuTrash2,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { BackgroundPlugin } from '@s4wave/core/session/session.pb.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import { useBackgroundPlugins } from '@s4wave/app/session/useBackgroundPlugins.js'
import {
  SpaceContentsContext,
  SpaceContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { PluginLifecycleBadge } from '@s4wave/web/sdk/app/lifecycle.js'
import { isValidSpacePluginId } from '@s4wave/core/space/world/world.js'

import { Button } from '@s4wave/web/ui/button.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { Input } from '@s4wave/web/ui/input.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import {
  SpaceContentsState,
  WatchSpaceContentsStateRequest,
} from '@s4wave/sdk/space/space.pb.js'

import { SpacePluginBuild } from './SpacePluginBuild.js'
import { SpacePluginImport } from './SpacePluginImport.js'

import { KNOWN_SPACE_PLUGINS, knownSpacePlugin } from './known-plugins.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
  },
  rule4: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule5: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule6: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule7: { cursor: 'not-allowed', opacity: 0.5 },
  rule8: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule9: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule12: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule13: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule14: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule15: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule16: { display: 'flex', minWidth: 0, flexDirection: 'column' },
  rule17: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule18: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule19: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule20: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule21: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
    WebkitUserSelect: 'all',
    userSelect: 'all',
  },
  rule22: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule23: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule24: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule26: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 90%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule27: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule28: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule29: {
    minWidth: 0,
    flex: 1,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule30: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: '#0000',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule31: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule32: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule33: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule34: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule35: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({ rule1: { flex: 1 } })

// CatalogEntry is one browsable plugin row merging the backend catalog with
// known-plugin presentation metadata (name, icon, fallback description).
interface CatalogEntry {
  id: string
  name: string
  description: string
  icon: IconType
  revision: string
}

// catalogSuggestions merges the backend plugin catalog with known-plugin
// presentation metadata, keyed by manifest ID, and returns the entries not yet
// installed sorted by name. The streamed availablePlugins list is the source
// of truth for what is installable; known-plugins supplies name/icon and a
// fallback description, and stands in as the browsable set until the catalog
// syncs.
function catalogSuggestions(
  available: NonNullable<SpaceContentsState['availablePlugins']>,
  installedIds: Set<string>,
): CatalogEntry[] {
  const catalogById = new Map<string, CatalogEntry>()
  for (const known of KNOWN_SPACE_PLUGINS) {
    catalogById.set(known.id, {
      id: known.id,
      name: known.name,
      description: known.description,
      icon: known.icon,
      revision: '',
    })
  }
  for (const plugin of available) {
    const id = plugin.pluginId ?? ''
    if (!id) continue
    const meta = knownSpacePlugin(id)
    catalogById.set(id, {
      id,
      name: meta.name,
      description: plugin.description || meta.description,
      icon: meta.icon,
      revision: plugin.revision ?? '',
    })
  }
  return Array.from(catalogById.values())
    .filter((entry) => !installedIds.has(entry.id))
    .sort((a, b) => a.name.localeCompare(b.name))
}

// BackgroundChoice is the saved background choice for an installed plugin
// that can run in the background.
type BackgroundChoice = 'unconfirmed' | 'running' | 'suspended'

// backgroundChoices maps each background plugin of the Space to the Session's
// saved choice for it.
function backgroundChoices(
  available: NonNullable<SpaceContentsState['availablePlugins']>,
  entries: BackgroundPlugin[],
  spaceId: string,
): Map<string, BackgroundChoice> {
  const choices = new Map<string, BackgroundChoice>()
  for (const plugin of available) {
    if (plugin.background && plugin.pluginId) {
      choices.set(plugin.pluginId, 'unconfirmed')
    }
  }
  for (const entry of entries) {
    const id = entry.pluginId ?? ''
    if (entry.spaceId !== spaceId || !choices.has(id)) continue
    choices.set(id, entry.suspended ? 'suspended' : 'running')
  }
  return choices
}

interface SpacePluginAddPanelProps {
  draftId: string
  draftError: string | null
  pending: string
  canSubmitDraft: boolean
  trimmedDraft: string
  suggestions: CatalogEntry[]
  onDraftChange: (value: string) => void
  onAdd: (pluginId: string) => void
  children?: ReactNode
}

// SpacePluginAddPanel renders manifest entry, catalog suggestions, and its
// children while the parent retains mutation and lifecycle state.
function SpacePluginAddPanel({
  draftId,
  draftError,
  pending,
  canSubmitDraft,
  trimmedDraft,
  suggestions,
  onDraftChange,
  onAdd,
  children,
}: SpacePluginAddPanelProps) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'border-foreground/6 bg-background-card/30 space-y-3 rounded-lg border p-3.5'
      }
    >
      <div>
        <label
          htmlFor="space-plugin-id"
          sx={[nativeChildSpacing.s1_5, stylexStyles4.rule1]}
        >
          Plugin manifest ID
        </label>
        <div sx={[nativeChildSpacing.s1_5, stylexStyles4.rule2]}>
          <Input
            id="space-plugin-id"
            value={draftId}
            onChange={(event) => onDraftChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && canSubmitDraft) onAdd(trimmedDraft)
            }}
            aria-invalid={draftError != null}
            placeholder="spacewave-notes"
            disabled={!!pending}
            variant="plugin"
            sx={stylexStyles.rule1}
          />
          <Button
            type="button"
            onClick={() => onAdd(trimmedDraft)}
            disabled={!canSubmitDraft}
            variant="brandOutline"
            size="sm"
          >
            {pending !== '' && pending === trimmedDraft ? (
              <LuLoaderCircle sx={stylexStyles2.rule1} />
            ) : (
              'Add'
            )}
          </Button>
        </div>
        {draftError && (
          <p sx={[nativeChildSpacing.s1_5, stylexStyles4.rule3]}>
            {draftError}
          </p>
        )}
      </div>

      {suggestions.length > 0 && (
        <div>
          <span sx={[nativeChildSpacing.s1_5, stylexStyles4.rule4]}>
            Available plugins
          </span>
          <div sx={[nativeChildSpacing.s1_5, stylexStyles4.rule5]}>
            {suggestions.map((plugin) => {
              const Icon = plugin.icon
              const isPending = pending === plugin.id
              return (
                <button
                  key={plugin.id}
                  type="button"
                  onClick={() => onAdd(plugin.id)}
                  disabled={!!pending}
                  sx={[stylexStyles4.rule6, !!pending && stylexStyles4.rule7]}
                >
                  <span sx={stylexStyles4.rule8}>
                    {isPending ? (
                      <LuLoaderCircle sx={stylexStyles2.rule2} />
                    ) : (
                      <Icon sx={stylexStyles3.rule1} />
                    )}
                  </span>
                  <span sx={stylexStyles4.rule9}>
                    <span sx={stylexStyles4.rule10}>
                      <span sx={stylexStyles4.rule11}>{plugin.name}</span>
                      {plugin.revision && (
                        <span sx={stylexStyles4.rule12}>
                          rev {plugin.revision}
                        </span>
                      )}
                    </span>
                    <span sx={stylexStyles4.rule13}>{plugin.description}</span>
                  </span>
                  <LuPlus sx={stylexStyles2.rule3} />
                </button>
              )
            })}
          </div>
        </div>
      )}

      {children}
    </div>
  )
}

interface RequestedPluginListProps {
  requestedIds: string[]
  availableIds: Set<string>
  pending: string
  onAdd: (pluginId: string) => void
}

// RequestedPluginList renders plugins something in the Space is waiting to
// load but the Space does not list. A plugin whose manifest is stored in the
// Space can be added in place; otherwise the row names the deploy step.
function RequestedPluginList({
  requestedIds,
  availableIds,
  pending,
  onAdd,
}: RequestedPluginListProps) {
  return (
    <>
      {requestedIds.map((id) => {
        const stored = availableIds.has(id)
        const isPending = pending === id
        return (
          <div key={id} sx={stylexStyles4.rule14}>
            <div sx={stylexStyles4.rule15}>
              <div sx={stylexStyles4.rule16}>
                <span sx={stylexStyles4.rule17}>{id}</span>
                <span sx={stylexStyles4.rule18}>
                  {stored
                    ? 'Waiting to load. Add it to this Space to continue.'
                    : 'Waiting to load. Add it from another Space, or deploy it here.'}
                </span>
              </div>
              <div sx={stylexStyles4.rule19}>
                <span sx={stylexStyles4.rule20}>Requested</span>
                {stored && (
                  <Button
                    type="button"
                    variant="brandOutline"
                    size="sm"
                    onClick={() => onAdd(id)}
                    disabled={!!pending}
                    aria-label={`Add ${id}`}
                  >
                    {isPending ? (
                      <LuLoaderCircle sx={stylexStyles2.rule1} />
                    ) : (
                      'Add'
                    )}
                  </Button>
                )}
              </div>
            </div>
            {!stored && (
              <code sx={stylexStyles4.rule21}>
                spacewave space deploy --manifest-id {id}
              </code>
            )}
          </div>
        )
      })}
    </>
  )
}

interface InstalledPluginListProps {
  plugins: NonNullable<SpaceContentsState['plugins']>
  pending: string
  confirmingRemoveId: string
  background: Map<string, BackgroundChoice>
  askingBackgroundId: string
  onRequestRemove: (pluginId: string) => void
  onCancelRemove: () => void
  onConfirmRemove: (pluginId: string) => void
  onAskBackground: (pluginId: string) => void
  onDismissBackground: () => void
  onAllowBackground: (pluginId: string) => void
}

// InstalledPluginList renders plugin lifecycle state, the background choice,
// and the inline removal confirmation without owning mutation state.
function InstalledPluginList({
  plugins,
  pending,
  confirmingRemoveId,
  background,
  askingBackgroundId,
  onRequestRemove,
  onCancelRemove,
  onConfirmRemove,
  onAskBackground,
  onDismissBackground,
  onAllowBackground,
}: InstalledPluginListProps) {
  if (plugins.length === 0) {
    return (
      <div sx={stylexStyles4.rule22}>
        <LuPuzzle sx={stylexStyles2.rule4} />
        <span sx={stylexStyles4.rule23}>No plugins installed</span>
      </div>
    )
  }

  return (
    <div sx={stylexStyles4.rule24}>
      {plugins.map((plugin) => {
        const id = plugin.pluginId ?? ''
        const meta = knownSpacePlugin(id)
        const desc = plugin.description || meta.description
        const detail = plugin.detail ?? ''
        const confirming = confirmingRemoveId === id
        const isPending = pending === id
        const choice = background.get(id)

        if (confirming) {
          return (
            <div key={id} sx={stylexStyles4.rule25}>
              <span sx={stylexStyles4.rule26}>Remove {id}?</span>
              <div sx={stylexStyles4.rule27}>
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => onConfirmRemove(id)}
                  disabled={isPending}
                >
                  {isPending ? (
                    <LuLoaderCircle sx={stylexStyles2.rule1} />
                  ) : (
                    'Confirm'
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onCancelRemove}
                  disabled={isPending}
                >
                  Cancel
                </Button>
              </div>
            </div>
          )
        }

        if (askingBackgroundId === id && choice === 'unconfirmed') {
          return (
            <div key={id} sx={stylexStyles4.rule28}>
              <span sx={stylexStyles4.rule29}>
                {meta.name} can keep running while this Space is closed. Allow?
              </span>
              <div sx={stylexStyles4.rule27}>
                <Button
                  variant="brandOutline"
                  size="sm"
                  onClick={() => onAllowBackground(id)}
                  disabled={isPending}
                >
                  {isPending ? (
                    <LuLoaderCircle sx={stylexStyles2.rule1} />
                  ) : (
                    'Allow'
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onDismissBackground}
                  disabled={isPending}
                >
                  Not now
                </Button>
              </div>
            </div>
          )
        }

        return (
          <div key={id} sx={stylexStyles4.rule30}>
            <div sx={stylexStyles4.rule16}>
              <span sx={stylexStyles4.rule17}>{id}</span>
              {desc && <span sx={stylexStyles4.rule18}>{desc}</span>}
              {detail && <span sx={stylexStyles4.rule31}>{detail}</span>}
            </div>
            <div sx={stylexStyles4.rule19}>
              {choice === 'unconfirmed' && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onAskBackground(id)}
                  disabled={!!pending}
                >
                  Allow background
                </Button>
              )}
              {choice === 'running' && (
                <span sx={stylexStyles4.rule32}>Background</span>
              )}
              {choice === 'suspended' && (
                <span sx={stylexStyles4.rule33}>Suspended</span>
              )}
              <PluginLifecycleBadge plugin={plugin} />
              {plugin.account ? (
                <span
                  sx={stylexStyles4.rule33}
                  title="Installed on your account for every Space. Remove it from the account plugin list."
                >
                  Account
                </span>
              ) : (
                <button
                  type="button"
                  onClick={() => onRequestRemove(id)}
                  disabled={!!pending}
                  aria-label={`Remove ${id}`}
                  sx={[stylexStyles4.rule34, !!pending && stylexStyles4.rule7]}
                >
                  <LuTrash2 sx={stylexStyles2.rule5} />
                </button>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}

// SpacePlugins renders the plugin management UI for a space: the installed
// plugins with lifecycle badges, an add-by-manifest-ID flow with known-plugin
// suggestions, a per-row remove with inline confirmation, and the background
// prompt for plugins that can run while the Space is closed.
export function SpacePlugins() {
  const spaceResource = SpaceContext.useContext()
  const space = useResourceValue(spaceResource)
  const contentsResource = SpaceContentsContext.useContext()
  const contents = useResourceValue(contentsResource)
  const { spaceId } = SpaceContainerContext.useContext()
  const backgroundPlugins = useBackgroundPlugins()

  const contentsState = useWatchStateRpc(
    useCallback(
      (req: WatchSpaceContentsStateRequest, signal: AbortSignal) =>
        contents?.watchState(req, signal) ?? null,
      [contents],
    ),
    {},
    WatchSpaceContentsStateRequest.equals,
    SpaceContentsState.equals,
  )

  const plugins = contentsState?.plugins ?? []
  const installedIds = new Set(plugins.map((plugin) => plugin.pluginId ?? ''))
  const requestedIds = contentsState?.requestedPluginIds ?? []
  const available = contentsState?.availablePlugins ?? []
  const background = backgroundChoices(
    available,
    backgroundPlugins.entries,
    spaceId,
  )

  const [adding, setAdding] = useState(false)
  const [draftId, setDraftId] = useState('')
  // pending holds the plugin ID of an in-flight add/remove mutation so its row
  // and control disable while the RPC runs.
  const [pending, setPending] = useState('')
  // confirmingRemoveId holds the plugin ID whose remove row is showing the
  // inline confirm affordance.
  const [confirmingRemoveId, setConfirmingRemoveId] = useState('')
  // askingBackgroundId holds the plugin ID whose row asks whether it may keep
  // running while the Space is closed.
  const [askingBackgroundId, setAskingBackgroundId] = useState('')

  // handleAdd installs pluginId, first copying its builds from source when
  // the plugin comes from another Space.
  const handleAdd = useCallback(
    async (pluginId: string, source?: Space) => {
      if (!space || pending) return
      setPending(pluginId)
      try {
        if (source) await space.importSpacePlugin(source.id, pluginId)
        await space.addSpacePlugin(pluginId)
        setDraftId('')
        setAdding(false)
        if (background.get(pluginId) === 'unconfirmed') {
          setAskingBackgroundId(pluginId)
        }
      } catch (err) {
        toast.error('Failed to add plugin', { description: String(err) })
      } finally {
        setPending('')
      }
    },
    [space, pending, background],
  )

  const handleRemove = useCallback(
    async (pluginId: string) => {
      if (!space || pending) return
      setPending(pluginId)
      try {
        await space.removeSpacePlugin(pluginId)
        setConfirmingRemoveId('')
        const choice = background.get(pluginId)
        if (choice === 'running' || choice === 'suspended') {
          await backgroundPlugins.set(spaceId, pluginId, false, false)
        }
      } catch (err) {
        toast.error('Failed to remove plugin', { description: String(err) })
      } finally {
        setPending('')
      }
    },
    [space, pending, background, backgroundPlugins, spaceId],
  )

  const handleAllowBackground = useCallback(
    async (pluginId: string) => {
      if (pending) return
      setPending(pluginId)
      try {
        await backgroundPlugins.set(spaceId, pluginId, true, false)
        setAskingBackgroundId('')
      } catch (err) {
        toast.error('Failed to allow background', {
          description: String(err),
        })
      } finally {
        setPending('')
      }
    },
    [pending, backgroundPlugins, spaceId],
  )

  const trimmedDraft = draftId.trim()
  const draftValid = isValidSpacePluginId(trimmedDraft)
  const draftDuplicate = installedIds.has(trimmedDraft)
  const draftError =
    trimmedDraft.length === 0
      ? null
      : draftDuplicate
        ? 'Plugin already installed'
        : !draftValid
          ? 'Use a lowercase manifest ID (letters, digits, dashes)'
          : null
  const canSubmitDraft = draftValid && !draftDuplicate && !pending

  const suggestions = catalogSuggestions(available, installedIds)

  return (
    <div sx={stylexStyles4.rule24}>
      <div sx={stylexStyles4.rule35}>
        <span sx={stylexStyles4.rule4}>{plugins.length} installed</span>
        <DashboardButton
          icon={
            adding ? (
              <LuX sx={stylexStyles2.rule5} />
            ) : (
              <LuPlus sx={stylexStyles2.rule5} />
            )
          }
          onClick={() => {
            setAdding((open) => !open)
            setDraftId('')
          }}
          aria-label={adding ? 'Cancel adding plugin' : 'Add plugin'}
        >
          {adding ? 'Cancel' : 'Add'}
        </DashboardButton>
      </div>

      {adding && (
        <SpacePluginAddPanel
          draftId={draftId}
          draftError={draftError}
          pending={pending}
          canSubmitDraft={canSubmitDraft}
          trimmedDraft={trimmedDraft}
          suggestions={suggestions}
          onDraftChange={setDraftId}
          onAdd={(pluginId) => void handleAdd(pluginId)}
        >
          <SpacePluginImport
            spaceId={spaceId}
            installedIds={installedIds}
            pending={pending}
            onImport={(source, pluginId) => void handleAdd(pluginId, source)}
          />
        </SpacePluginAddPanel>
      )}

      <SpacePluginBuild space={space} />

      <RequestedPluginList
        requestedIds={requestedIds}
        availableIds={new Set(available.map((plugin) => plugin.pluginId ?? ''))}
        pending={pending}
        onAdd={(pluginId) => void handleAdd(pluginId)}
      />

      <InstalledPluginList
        plugins={plugins}
        pending={pending}
        confirmingRemoveId={confirmingRemoveId}
        background={background}
        askingBackgroundId={askingBackgroundId}
        onRequestRemove={setConfirmingRemoveId}
        onCancelRemove={() => setConfirmingRemoveId('')}
        onConfirmRemove={(pluginId) => void handleRemove(pluginId)}
        onAskBackground={setAskingBackgroundId}
        onDismissBackground={() => setAskingBackgroundId('')}
        onAllowBackground={(pluginId) => void handleAllowBackground(pluginId)}
      />
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
