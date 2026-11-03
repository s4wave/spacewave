import { forgeControlStyles } from '@s4wave/web/style/forge-controls.stylex.js'
import { useCallback, type ReactNode } from 'react'

import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import type { ForgeLinkedEntity } from './useForgeLinkedEntities.js'

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule2: {
    padding: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: { flexShrink: 0 },
  rule4: {
    '--tw-duration': '.15s',
    borderColor: {
      default: '#0000',
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
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
    borderBottomLeftRadius: {
      default: null,
      ':last-child': themeRadii['--radius-lg'],
    },
    borderBottomRightRadius: {
      default: null,
      ':last-child': themeRadii['--radius-lg'],
    },
    borderTopLeftRadius: {
      default: null,
      ':first-child': themeRadii['--radius-lg'],
    },
    borderTopRightRadius: {
      default: null,
      ':first-child': themeRadii['--radius-lg'],
    },
    width: '100%',
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
  },
  rule6: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
})

interface ForgeEntityListProps {
  entities: ForgeLinkedEntity[]
  loading: boolean
  icon: ReactNode
  loadingLabel?: string
  emptyLabel?: string

  sx?: stylex.StyleXStyles
  className?: never
}

// ForgeEntityList renders a compact dense list of forge entities with a
// loading inline spinner and a compact single-line empty state.
export function ForgeEntityList({
  entities,
  loading,
  icon,
  loadingLabel = 'Loading linked entities',
  emptyLabel = 'None linked yet',
  sx,
}: ForgeEntityListProps) {
  if (loading && entities.length === 0) {
    return (
      <div sx={[stylexStyles.rule1, sx]}>
        <div sx={stylexStyles.rule2}>
          <LoadingInline label={loadingLabel} size="sm" tone="subtle" />
        </div>
      </div>
    )
  }
  if (entities.length === 0) {
    return (
      <div sx={[stylexStyles.rule1, sx]}>
        <div sx={stylexStyles.rule2}>
          <span sx={stylexStyles.rule3}>{icon}</span>
          <span>{emptyLabel}</span>
        </div>
      </div>
    )
  }
  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      {entities.map((entity) => (
        <DenseEntityRow key={entity.objectKey} entity={entity} icon={icon} />
      ))}
    </div>
  )
}

interface DenseEntityRowProps {
  entity: ForgeLinkedEntity
  icon: ReactNode
}

// DenseEntityRow renders a single compact row (icon + key + navigate-on-click).
function DenseEntityRow({ entity, icon }: DenseEntityRowProps) {
  const container = SpaceContainerContext.useContextSafe()
  const onClick = useCallback(() => {
    container?.navigateToObjects([entity.objectKey])
  }, [container, entity.objectKey])

  if (!container) {
    return (
      <div sx={[stylexStyles.rule4]}>
        <span sx={stylexStyles.rule5}>{icon}</span>
        <span sx={stylexStyles.rule6}>{entity.objectKey}</span>
      </div>
    )
  }
  return (
    <button
      type="button"
      onClick={onClick}
      sx={[[stylexStyles.rule4], forgeControlStyles.required]}
    >
      <span sx={stylexStyles.rule5}>{icon}</span>
      <span sx={stylexStyles.rule6}>{entity.objectKey}</span>
    </button>
  )
}
