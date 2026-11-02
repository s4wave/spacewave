import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  iconDefaults,
  iconRequired,
  traceIconMarker,
} from '../style/icon-defaults.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import * as React from 'react'
import { LuChevronRight } from '@s4wave/web/images/icons.js'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@s4wave/web/ui/collapsible.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  radii as themeRadii,
  colors as themeColors,
  motion as themeMotion,
} from '../style/tokens.stylex.js'
import {
  traceTriggerGroupMarker,
  traceLineMarker,
  traceTitleMarker,
} from '../style/relations.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    minWidth: 0,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    display: 'flex',
    width: 'fit-content',
    maxWidth: '100%',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-muted-foreground'],
      ':is([data-status="error"])': themeColors['--color-destructive'],
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':is([data-status="pending"])': 0.6 },
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule4: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule5: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-muted-foreground']} 70%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule6: {
    display: 'flex',
    width: '100%',
    minWidth: 0,
    flexDirection: 'column',
  },
  rule7: {
    minWidth: 0,
    overflow: 'hidden',
    '--tw-duration': '.2s',
    transitionDuration: '.2s',
    '--radix-accordion-content-height':
      'var(--radix-collapsible-content-height)',
  },
  rule8: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    marginBottom: themeLayout['--spacing'],
    marginLeft: `calc(calc(${themeLayout['--spacing']} * 2) - 1px)`,
    minWidth: 0,
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    paddingRight: themeLayout['--spacing'],
    paddingLeft: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-leading': 'var(--leading-relaxed)',
    lineHeight: 'var(--leading-relaxed)',
    overflowWrap: 'break-word',
    color: themeColors['--color-muted-foreground'],
  },
})

const triggerStyles = stylex.create({
  trigger: {
    display: 'flex',
    width: 'fit-content',
    maxWidth: '100%',
    minWidth: 0,
    cursor: 'pointer',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    textAlign: 'left',
    color: {
      default: themeColors['--color-muted-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
      ':has([data-status="error"])': themeColors['--color-destructive'],
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
  },
})

const indicatorStyles = stylex.create({
  indicator: {
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    rotate: {
      default: null,
      [stylex.when.ancestor(
        ':is([aria-expanded="true"])',
        traceTriggerGroupMarker,
      )]: '90deg',
    },
  },
})

const effectStyles = stylex.create({
  title: {
    backgroundImage: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        {
          default:
            'linear-gradient(110deg, currentColor 40%, color-mix(in oklch, currentColor 20%, transparent) 50%, currentColor 60%)',
          '@media (prefers-reduced-motion: reduce)': 'none',
        },
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    WebkitTextFillColor: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        {
          default: 'transparent',
          '@media (prefers-reduced-motion: reduce)': 'currentColor',
        },
      '@media (prefers-reduced-motion: reduce)': 'currentColor',
    },
    backgroundRepeat: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        'no-repeat',
    },
    backgroundSize: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        '200% 100%',
    },
    WebkitBackgroundClip: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        'text',
    },
    backgroundClip: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        'text',
    },
    animationName: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        {
          default: 'tw-shimmer',
          '@media (prefers-reduced-motion: reduce)': 'none',
        },
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationDuration: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: '2s', '@media (prefers-reduced-motion: reduce)': '0s' },
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationTimingFunction: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        {
          default: 'linear',
          '@media (prefers-reduced-motion: reduce)': 'ease',
        },
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    animationIterationCount: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: 'infinite', '@media (prefers-reduced-motion: reduce)': 1 },
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationDirection: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        {
          default: 'normal',
          '@media (prefers-reduced-motion: reduce)': 'normal',
        },
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationPlayState: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        {
          default: 'running',
          '@media (prefers-reduced-motion: reduce)': 'running',
        },
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationDelay: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: '0s', '@media (prefers-reduced-motion: reduce)': '0s' },
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: 'none', '@media (prefers-reduced-motion: reduce)': 'none' },
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationTimeline: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: 'auto', '@media (prefers-reduced-motion: reduce)': 'auto' },
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
  },
  content: {
    animationName: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        'accordion-down',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'accordion-up',
    },
    animationDuration: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))": '.2s',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        '.2s',
    },
    animationTimingFunction: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        'ease-out',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'ease-out',
    },
    animationIterationCount: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))": 1,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))": 1,
    },
    animationDirection: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        'normal',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'normal',
    },
    animationPlayState: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        'running',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'running',
    },
    animationDelay: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))": '0s',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        '0s',
    },
    animationFillMode: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))": 'none',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'none',
    },
    animationTimeline: {
      default: null,
      ":is([data-state='open'], [data-open]:not([data-open='false']))": 'auto',
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'auto',
    },
  },
})

const relationshipStyles = stylex.create({
  line: {
    cursor: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceTriggerGroupMarker)]: 'pointer',
    },
    color: {
      default: themeColors['--color-muted-foreground'],
      [stylex.when.ancestor(':hover', traceTriggerGroupMarker)]:
        themeColors['--color-foreground'],
    },
  },
  errorLine: {
    cursor: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceTriggerGroupMarker)]: 'pointer',
    },
    color: {
      default: themeColors['--color-destructive'],
      [stylex.when.ancestor(':hover', traceTriggerGroupMarker)]:
        themeColors['--color-destructive'],
    },
  },
  icon: {
    marginLeft: {
      default: null,
      [stylex.when.siblingBefore(':is(*)', traceTitleMarker)]:
        `calc(${themeLayout['--spacing']} * -1)`,
    },
  },
})

/** traceLinkStyles gives a source link its own trace geometry and focus ring. */
export const traceLinkStyles = stylex.create({
  link: {
    display: 'flex',
    width: 'fit-content',
    maxWidth: '100%',
    minWidth: 0,
    borderRadius: themeRadii['--radius-md'],
    textAlign: 'left',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
  },
})

/** TraceStatus is the live state of one trace line. Done is the settled state. */
export type TraceStatus = 'running' | 'done' | 'error' | 'pending'

/** Trace is a list of agent steps. */
function Trace({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div data-slot="trace" {...stylexProps([stylexStyles.rule1, sx], props)} />
  )
}

/** TraceLine is one step, with an optional running, done, error, or pending status. */
function TraceLine({
  sx,
  status,
  ...props
}: Omit<
  React.ComponentProps<'span'> & {
    status?: TraceStatus
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <span
      data-slot="trace-line"
      data-status={status}
      {...stylexProps(
        [
          traceLineMarker,
          stylexStyles.rule2,
          status === 'error'
            ? relationshipStyles.errorLine
            : relationshipStyles.line,
          sx,
        ],
        props,
      )}
    />
  )
}

/** TraceIcon holds the step icon. */
function TraceIcon({
  sx,
  ...props
}: Omit<React.ComponentProps<'span'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <IconStylesProvider
      sx={[iconDefaults.trace]}
      requiredSx={iconRequired.trace}
    >
      <span
        data-slot="trace-icon"
        aria-hidden="true"
        {...stylexProps(
          [traceIconMarker, stylexStyles.rule3, relationshipStyles.icon, sx],
          props,
        )}
      />
    </IconStylesProvider>
  )
}

/** TraceTitle is the step name. A running line shimmers this text. */
function TraceTitle({
  sx,
  ...props
}: Omit<React.ComponentProps<'span'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <span
      data-slot="trace-title"
      {...stylexProps(
        [traceTitleMarker, stylexStyles.rule4, effectStyles.title, sx],
        props,
      )}
    />
  )
}

/** TraceDescription is the optional detail beside the title. */
function TraceDescription({
  sx,
  ...props
}: Omit<React.ComponentProps<'span'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <span
      data-slot="trace-description"
      {...stylexProps([stylexStyles.rule5, sx], props)}
    />
  )
}

/** TraceCollapsible nests the steps revealed by its trigger. */
function TraceCollapsible({
  sx,
  ...props
}: Omit<
  Omit<React.ComponentProps<typeof Collapsible>, 'asChild' | 'open'> &
    (
      | { open?: undefined }
      | {
          open: boolean
          onOpenChange: NonNullable<
            React.ComponentProps<typeof Collapsible>['onOpenChange']
          >
        }
    ),
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <Collapsible
      data-slot="trace-collapsible"
      {...stylexProps([stylexStyles.rule6, sx], props)}
    />
  )
}

/** TraceCollapsibleTrigger toggles the nested steps and shows the disclosure icon. */
function TraceCollapsibleTrigger({
  sx,
  children,
  ...props
}: Omit<
  React.ComponentProps<typeof CollapsibleTrigger>,
  'asChild' | 'className'
> & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <CollapsibleTrigger
      data-slot="trace-collapsible-trigger"
      {...stylexProps(
        [[triggerStyles.trigger, traceTriggerGroupMarker], sx],
        props,
      )}
    >
      {children}
      <TraceIcon sx={indicatorStyles.indicator}>
        <LuChevronRight />
      </TraceIcon>
    </CollapsibleTrigger>
  )
}

/** TraceCollapsibleContent indents the nested steps under the trigger. */
function TraceCollapsibleContent({
  sx,
  children,
  ...props
}: Omit<
  Omit<React.ComponentProps<typeof CollapsibleContent>, 'asChild'>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <CollapsibleContent
      data-slot="trace-collapsible-content"
      {...stylexProps([stylexStyles.rule7, effectStyles.content], props)}
    >
      <div
        data-slot="trace-collapsible-content-inner"
        sx={[stylexStyles.rule8, sx]}
      >
        {children}
      </div>
    </CollapsibleContent>
  )
}

export {
  Trace,
  TraceLine,
  TraceIcon,
  TraceTitle,
  TraceDescription,
  TraceCollapsible,
  TraceCollapsibleTrigger,
  TraceCollapsibleContent,
}
