import { keyframeNames } from '../../web/style/tokens.stylex.js'
import { useCallback, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { LuLoaderCircle, LuPlus } from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  useSharedObjectBodyMount,
  useSharedObjectMount,
  useSpaceContentsMount,
  useSpaceMount,
} from '@s4wave/app/hooks/useSharedMounts.js'
import { useWatchSpacesList } from '@s4wave/app/system/useSystemStatus.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

import type { Space } from '@s4wave/sdk/space/space.js'
import {
  SpaceContentsState,
  WatchSpaceContentsStateRequest,
} from '@s4wave/sdk/space/space.pb.js'

import { knownSpacePlugin } from './known-plugins.js'

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

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.6rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule3: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 16%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-foreground'],
    cursor: 'not-allowed',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: 0.5,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 16%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule5: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 16%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-foreground-alt'],
    cursor: 'not-allowed',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: 0.5,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule6: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 16%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule7: {
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule11: {
    '--tw-duration': '.15s',
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule12: { cursor: 'not-allowed', opacity: 0.5 },
  rule13: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule14: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule15: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule17: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.6rem',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: 1.5,
  },
  rule18: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '1s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['spin'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'linear',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '1s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['spin'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'linear',
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface SourceSpace {
  id: string
  name: string
}

interface SpacePluginImportProps {
  // spaceId is the Space plugins are imported into; it is not offered as a source.
  spaceId: string
  installedIds: Set<string>
  pending: string
  onImport: (source: Space, pluginId: string) => void
}

// SpacePluginImport lists the session's other Spaces and, for the one picked,
// the plugins it stores that this Space has not installed. Adding one copies
// its builds into this Space and installs it.
export function SpacePluginImport({
  sx,
  spaceId,
  installedIds,
  pending,
  onImport,
}: SpacePluginImportProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const spacesList = useWatchSpacesList()
  const [sourceId, setSourceId] = useState('')

  const sources: SourceSpace[] = []
  for (const entry of spacesList ?? []) {
    const id = entry.entry?.ref?.providerResourceRef?.id ?? ''
    if (!id || id === spaceId) continue
    sources.push({ id, name: entry.spaceMeta?.name || id })
  }
  if (sources.length === 0) return null
  const source = sources.find((space) => space.id === sourceId)

  return (
    <div sx={sx}>
      <span sx={[nativeChildSpacing.s1_5, stylexStyles3.rule1]}>
        From another Space
      </span>
      <div sx={[nativeChildSpacing.s1_5, stylexStyles3.rule2]}>
        {sources.map((space) => (
          <button
            key={space.id}
            type="button"
            aria-pressed={space.id === sourceId}
            onClick={() => setSourceId(space.id === sourceId ? '' : space.id)}
            disabled={!!pending}
            sx={
              space.id === sourceId
                ? pending
                  ? stylexStyles3.rule3
                  : stylexStyles3.rule4
                : pending
                  ? stylexStyles3.rule5
                  : stylexStyles3.rule6
            }
          >
            {space.name}
          </button>
        ))}
      </div>
      {source && (
        <SourceSpaceCatalog
          sx={nativeChildSpacing.s1_5}
          key={source.id}
          source={source}
          installedIds={installedIds}
          pending={pending}
          onImport={onImport}
        />
      )}
    </div>
  )
}

interface SourceSpaceCatalogProps {
  source: SourceSpace
  installedIds: Set<string>
  pending: string
  onImport: (source: Space, pluginId: string) => void
}

// SourceSpaceCatalog mounts the source Space and lists the plugins it stores.
function SourceSpaceCatalog({
  sx,
  source,
  installedIds,
  pending,
  onImport,
}: SourceSpaceCatalogProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const sessionResource = SessionContext.useContext()
  const sharedObjectResource = useSharedObjectMount(sessionResource, source.id)
  const bodyResource = useSharedObjectBodyMount(sharedObjectResource)
  const spaceResource = useSpaceMount(bodyResource)
  const sourceSpace = useResourceValue(spaceResource)
  const contentsResource = useSpaceContentsMount(spaceResource)
  const contents = useResourceValue(contentsResource)
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

  const mountError =
    sharedObjectResource.error ??
    bodyResource.error ??
    spaceResource.error ??
    contentsResource.error
  if (mountError) {
    return (
      <p sx={[stylexStyles3.rule7, sx]}>
        Couldn't open {source.name}: {mountError.message}
      </p>
    )
  }
  if (!sourceSpace || !contentsState) {
    return (
      <p sx={[stylexStyles3.rule8, sx]}>
        <LuLoaderCircle sx={stylexStyles.rule1} />
        Reading the plugins in {source.name}
      </p>
    )
  }

  const plugins = (contentsState.availablePlugins ?? []).filter(
    (plugin) => plugin.pluginId && !installedIds.has(plugin.pluginId),
  )
  if (plugins.length === 0) {
    return (
      <p sx={[stylexStyles3.rule9, sx]}>
        {source.name} has no plugin builds this Space lacks.
      </p>
    )
  }

  return (
    <div sx={[stylexStyles3.rule10, sx]}>
      {plugins.map((plugin) => {
        const id = plugin.pluginId ?? ''
        const meta = knownSpacePlugin(id)
        const Icon = meta.icon
        return (
          <button
            key={id}
            type="button"
            onClick={() => onImport(sourceSpace, id)}
            disabled={!!pending}
            sx={[stylexStyles3.rule11, !!pending && stylexStyles3.rule12]}
          >
            <span sx={stylexStyles3.rule13}>
              {pending === id ? (
                <LuLoaderCircle sx={stylexStyles.rule2} />
              ) : (
                <Icon sx={stylexStyles2.rule1} />
              )}
            </span>
            <span sx={stylexStyles3.rule14}>
              <span sx={stylexStyles3.rule15}>
                <span sx={stylexStyles3.rule16}>{meta.name}</span>
                {plugin.revision && (
                  <span sx={stylexStyles3.rule17}>rev {plugin.revision}</span>
                )}
              </span>
              <span sx={stylexStyles3.rule18}>
                {plugin.description || meta.description}
              </span>
            </span>
            <LuPlus sx={stylexStyles.rule3} />
          </button>
        )
      })}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
  },
})
