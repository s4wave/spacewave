import {
  LuEyeOff,
  LuLocateFixed,
  LuPlus,
  LuTrash2,
} from '@s4wave/web/images/icons.js'

import type { EphemeralEdge } from './types.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: '.25rem',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    borderRadius: '.25rem',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    color: themeColors['--color-warning'],
  },
  rule3: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6.5)`,
    maxWidth: `calc(${themeLayout['--spacing']} * 64)`,
    alignItems: 'center',
  },
  rule4: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6.5)`,
    minWidth: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderTopLeftRadius: themeRadii['--radius-md'],
    borderBottomLeftRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: 0,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule7: {
    maxWidth: `calc(${themeLayout['--spacing']} * 24)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule8: {
    maxWidth: `calc(${themeLayout['--spacing']} * 20)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    marginLeft: 'auto',
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule10: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6.5)`,
    alignItems: 'center',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule11: {
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: 0,
  },
  rule12: {
    borderTopRightRadius: themeRadii['--radius-md'],
    borderBottomRightRadius: themeRadii['--radius-md'],
  },
  rule13: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6.5)`,
    alignItems: 'center',
    borderTopRightRadius: themeRadii['--radius-md'],
    borderBottomRightRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    color: themeColors['--color-destructive'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

interface GraphLinkPillProps {
  edge: EphemeralEdge
  loaded: boolean
  onPrimary: () => void
  onHide?: () => void
  onDelete?: () => void
}

interface GraphLinkBadgesProps {
  edge: EphemeralEdge
}

/** GraphLinkBadges flags a link as protected, capped, or partly hidden. */
function GraphLinkBadges({ edge }: GraphLinkBadgesProps) {
  const capped =
    (edge.direction === 'out' && edge.outgoingTruncated) ||
    (edge.direction === 'in' && edge.incomingTruncated)

  return (
    <>
      {edge.protected && <span sx={stylexStyles2.rule1}>protected</span>}
      {capped && <span sx={stylexStyles2.rule2}>capped</span>}
      {edge.hiddenCount > 0 && (
        <span sx={stylexStyles2.rule1}>hidden {edge.hiddenCount}</span>
      )}
    </>
  )
}

// GraphLinkPill renders the inline Canvas graph-link action pill.
export function GraphLinkPill({
  edge,
  loaded,
  onPrimary,
  onHide,
  onDelete,
}: GraphLinkPillProps) {
  return (
    <div sx={stylexStyles2.rule3}>
      <button
        type="button"
        sx={[stylexStyles2.rule4]}
        onClick={onPrimary}
        title={`${loaded ? 'Focus' : 'Load'} ${edge.linkedObjectLabel}`}
      >
        <span sx={stylexStyles2.rule5}>{edge.predicate}</span>
        <span sx={stylexStyles2.rule6}>/</span>
        <span sx={stylexStyles2.rule7}>{edge.linkedObjectLabel}</span>
        {edge.linkedObjectTypeLabel && (
          <span sx={stylexStyles2.rule8}>{edge.linkedObjectTypeLabel}</span>
        )}
        <GraphLinkBadges edge={edge} />
        <span sx={stylexStyles2.rule9}>
          {loaded ? (
            <LuLocateFixed sx={stylexStyles.rule1} />
          ) : (
            <LuPlus sx={stylexStyles.rule1} />
          )}
          {loaded ? 'Focus' : 'Load'}
        </span>
      </button>
      {edge.hideable && onHide && (
        <button
          type="button"
          sx={[
            stylexStyles2.rule10,
            edge.userRemovable && onDelete
              ? stylexStyles2.rule11
              : stylexStyles2.rule12,
          ]}
          onClick={onHide}
          title={`Hide ${edge.predicate} link`}
        >
          <LuEyeOff sx={stylexStyles.rule1} />
        </button>
      )}
      {edge.userRemovable && onDelete && (
        <button
          type="button"
          sx={stylexStyles2.rule13}
          onClick={onDelete}
          title={`Delete ${edge.predicate} link`}
        >
          <LuTrash2 sx={stylexStyles.rule1} />
        </button>
      )}
    </div>
  )
}
