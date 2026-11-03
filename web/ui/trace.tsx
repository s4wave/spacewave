import { keyframeNames } from '../style/tokens.stylex.js'
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
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-muted-foreground'],
      ':is([data-status="error"])': themeColors['--color-destructive'],
    },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':is([data-status="pending"])': 0.6 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    maxWidth: '100%',
    minWidth: 0,
    width: 'fit-content',
  },
  rule3: {
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule5: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-muted-foreground']} 70%, transparent)`,
    flexShrink: 0,
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule6: {
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    width: '100%',
  },
  rule7: {
    '--radix-accordion-content-height':
      'var(--radix-collapsible-content-height)',
    '--tw-duration': '.2s',
    overflow: 'hidden',
    transitionDuration: '.2s',
    minWidth: 0,
  },
  rule8: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-muted-foreground'],
    lineHeight: 'var(--leading-relaxed)',
    overflowWrap: 'break-word',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    marginBottom: themeLayout['--spacing'],
    marginLeft: `calc(calc(${themeLayout['--spacing']} * 2) - 1px)`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    minWidth: 0,
    paddingLeft: `calc(${themeLayout['--spacing']} * 4)`,
    paddingRight: themeLayout['--spacing'],
  },
})

const triggerStyles = stylex.create({
  trigger: {
    '--tw-outline-style': 'none',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: themeColors['--color-muted-foreground'],
      ':has([data-status="error"])': themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    outlineStyle: 'none',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    maxWidth: '100%',
    minWidth: 0,
    width: 'fit-content',
  },
})

const indicatorStyles = stylex.create({
  indicator: {
    rotate: {
      default: null,
      [stylex.when.ancestor(
        ':is([aria-expanded="true"])',
        traceTriggerGroupMarker,
      )]: '90deg',
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const effectStyles = stylex.create({
  title: {
    WebkitBackgroundClip: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        'text',
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
    animationDelay: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: '0s', '@media (prefers-reduced-motion: reduce)': '0s' },
      '@media (prefers-reduced-motion: reduce)': '0s',
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
    animationDuration: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: '2s', '@media (prefers-reduced-motion: reduce)': '0s' },
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: 'none', '@media (prefers-reduced-motion: reduce)': 'none' },
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationIterationCount: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: 'infinite', '@media (prefers-reduced-motion: reduce)': 1 },
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationName: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        {
          default: keyframeNames['tw-shimmer'],
          '@media (prefers-reduced-motion: reduce)': 'none',
        },
      '@media (prefers-reduced-motion: reduce)': 'none',
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
    animationTimeline: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        { default: 'auto', '@media (prefers-reduced-motion: reduce)': 'auto' },
      '@media (prefers-reduced-motion: reduce)': 'auto',
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
    backgroundClip: {
      default: null,
      [stylex.when.ancestor(':is([data-status="running"]) >', traceLineMarker)]:
        'text',
    },
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
  },
  content: {
    animationDelay: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        '0s',
      ":is([data-state='open'], [data-open]:not([data-open='false']))": '0s',
    },
    animationDirection: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'normal',
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        'normal',
    },
    animationDuration: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        '.2s',
      ":is([data-state='open'], [data-open]:not([data-open='false']))": '.2s',
    },
    animationFillMode: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'none',
      ":is([data-state='open'], [data-open]:not([data-open='false']))": 'none',
    },
    animationIterationCount: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))": 1,
      ":is([data-state='open'], [data-open]:not([data-open='false']))": 1,
    },
    animationName: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        keyframeNames['accordion-up'],
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        keyframeNames['accordion-down'],
    },
    animationPlayState: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'running',
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        'running',
    },
    animationTimeline: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'auto',
      ":is([data-state='open'], [data-open]:not([data-open='false']))": 'auto',
    },
    animationTimingFunction: {
      default: null,
      ":is([data-state='closed'], [data-closed]:not([data-closed='false']))":
        'ease-out',
      ":is([data-state='open'], [data-open]:not([data-open='false']))":
        'ease-out',
    },
  },
})

const relationshipStyles = stylex.create({
  line: {
    color: {
      default: themeColors['--color-muted-foreground'],
      [stylex.when.ancestor(':hover', traceTriggerGroupMarker)]:
        themeColors['--color-foreground'],
    },
    cursor: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceTriggerGroupMarker)]: 'pointer',
    },
  },
  errorLine: {
    color: {
      default: themeColors['--color-destructive'],
      [stylex.when.ancestor(':hover', traceTriggerGroupMarker)]:
        themeColors['--color-destructive'],
    },
    cursor: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceTriggerGroupMarker)]: 'pointer',
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
    '--tw-outline-style': 'none',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    display: 'flex',
    outlineStyle: 'none',
    textAlign: 'left',
    maxWidth: '100%',
    minWidth: 0,
    width: 'fit-content',
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
