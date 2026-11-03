import * as ownerStylex from '@stylexjs/stylex'

import React from 'react'

import {
  layout as themeLayout,
  radii as themeRadii,
  colors as themeColors,
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker4 } from '../../web/style/relations.stylex.js'

const stylexStyles2 = ownerStylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    cursor: 'not-allowed',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    opacity: 0.5,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    display: 'flex',
    width: '100%',
    cursor: 'not-allowed',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    opacity: 0.5,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: {
    display: 'flex',
    width: '100%',
    cursor: 'not-allowed',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    opacity: 0.5,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule6: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    display: 'flex',
    width: '100%',
    cursor: 'not-allowed',
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
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    opacity: 0.5,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule8: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
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
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = ownerStylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: { color: themeColors['--color-foreground-alt'] },
  rule6: { color: themeColors['--color-destructive'] },
  rule7: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule9: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

// ActionCard renders an interactive card with icon, label, and description.
export interface ActionCardProps {
  icon: React.ReactNode
  label: string
  description: string
  onClick?: () => void
  variant?: 'default' | 'destructive'
  // Tightens padding and drops the description for a narrow container.
  compact?: boolean
}

export function ActionCard({
  icon,
  label,
  description,
  onClick,
  variant = 'default',
  compact = false,
}: ActionCardProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!onClick}
      sx={actionCardStyles(compact, variant === 'destructive', !onClick)}
    >
      <div
        sx={
          compact
            ? variant === 'destructive'
              ? stylexStyles.rule1
              : stylexStyles.rule2
            : variant === 'destructive'
              ? stylexStyles.rule3
              : stylexStyles.rule4
        }
      >
        <div
          sx={[
            stylexStyles.rule5,
            variant === 'destructive' && stylexStyles.rule6,
          ]}
        >
          {icon}
        </div>
      </div>
      <div sx={stylexStyles.rule7}>
        <h4 sx={stylexStyles.rule8}>{label}</h4>
        {!compact && <p sx={stylexStyles.rule9}>{description}</p>}
      </div>
    </button>
  )
}

// actionCardStyles selects one complete hover and disabled recipe for the card.
function actionCardStyles(
  compact: boolean,
  destructive: boolean,
  disabled: boolean,
): ownerStylex.StyleXStyles {
  return compact
    ? destructive
      ? disabled
        ? [stylexStyles2.rule1, groupMarker4]
        : [stylexStyles2.rule2, groupMarker4]
      : disabled
        ? [stylexStyles2.rule3, groupMarker4]
        : [stylexStyles2.rule4, groupMarker4]
    : destructive
      ? disabled
        ? [stylexStyles2.rule5, groupMarker4]
        : [stylexStyles2.rule6, groupMarker4]
      : disabled
        ? [stylexStyles2.rule7, groupMarker4]
        : [stylexStyles2.rule8, groupMarker4]
}
