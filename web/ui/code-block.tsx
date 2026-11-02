import { ControlStylesProvider } from '@s4wave/web/style/ControlStylesProvider.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  iconDefaults,
  iconRequired,
  codeTitleIconMarker,
} from '../style/icon-defaults.stylex.js'
import * as React from 'react'
import { type VariantProps } from 'class-variance-authority'
import { LuCheck, LuCopy } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'

import * as stylex from '@stylexjs/stylex'

import {
  radii as themeRadii,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
  layout as themeLayout,
  motion as themeMotion,
} from '../style/tokens.stylex.js'
import {
  codeBlockGroupMarker,
  codeGroupMarker,
} from '../style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    width: '100%',
    minWidth: 0,
    flexDirection: 'column',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    fontSize: {
      default: themeFonts['--text-sm'],
      ':is([data-size="sm"])': themeFonts['--text-xs'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      ':is([data-size="sm"])': `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
  },
  rule2: {
    position: 'relative',
    display: 'flex',
    width: '100%',
    minWidth: 0,
    flexDirection: 'column',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
    },
    fontSize: {
      default: themeFonts['--text-sm'],
      ':is([data-size="sm"])': themeFonts['--text-xs'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      ':is([data-size="sm"])': `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    },
    color: themeColors['--color-destructive'],
    '--tw-ring-shadow': {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
  },
  rule3: {
    position: 'relative',
    display: 'flex',
    width: '100%',
    minWidth: 0,
    flexDirection: 'column',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: themeColors['--color-input'],
      ':has( > [data-slot="code-block-content"]:focus-visible)':
        themeColors['--color-ring'],
    },
    fontSize: {
      default: themeFonts['--text-sm'],
      ':is([data-size="sm"])': themeFonts['--text-xs'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      ':is([data-size="sm"])': `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    },
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':has( > [data-slot="code-block-content"]:focus-visible)': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
    backgroundColor: {
      default: null,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
  },
  rule4: {
    display: 'flex',
    minWidth: 0,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    paddingTop: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    color: 'var(--code-block-header-color, var(--color-muted-foreground))',
  },
  rule5: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: 'var(--font-sans)',
    fontSize: 'var(--code-block-title-size, var(--text-sm))',
    lineHeight:
      'var(--tw-leading, var(--code-block-title-leading, var(--text-sm--line-height)))',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: 'var(--code-block-title-color, var(--color-foreground))',
  },
  rule6: {
    marginBlock: {
      default: `calc(${themeLayout['--spacing']} * -1)`,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 0,
    },
    marginInlineStart: {
      default: 'auto',
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 0,
    },
    marginInlineEnd: {
      default: `calc(${themeLayout['--spacing']} * -1.5)`,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 0,
    },
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    position: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 'absolute',
    },
    insetInlineEnd: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]:
        'var(--code-block-action-inset, calc(var(--spacing) * 2))',
    },
    top: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]:
        'var(--code-block-action-inset, calc(var(--spacing) * 2))',
    },
    zIndex: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 10,
    },

    opacity: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 0,
      [stylex.when.ancestor(':focus-within >', codeBlockGroupMarker)]: 1,
      '@media (pointer: coarse)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 1,
      },
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':hover >', codeBlockGroupMarker)]: 1,
      },
    },
    transitionProperty: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]: 'opacity',
    },
    transitionTimingFunction: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]:
        `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    },
    transitionDuration: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeBlockGroupMarker)]:
        'var(--tw-duration, var(--default-transition-duration))',
    },
  },
  rule7: {
    minHeight: 0,
    minWidth: 0,
    scrollbarWidth: 'thin',
    overflow: 'auto',
    WebkitMaskImage: {
      default: 'linear-gradient(to right, #0000, #000 12% 88%, #0000)',
      ':is([data-wrap])': 'linear-gradient(#0000, #000 12% 88%, #0000)',
    },
    maskImage: {
      default: 'linear-gradient(to right, #0000, #000 12% 88%, #0000)',
      ':is([data-wrap])': 'linear-gradient(#0000, #000 12% 88%, #0000)',
    },
    paddingInline: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    paddingTop: 'var(--code-block-content-top, calc(var(--spacing) * 3))',
    paddingBottom: 'var(--code-block-content-bottom, calc(var(--spacing) * 3))',
    textAlign: 'start',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    '--tw-leading': 'var(--leading-relaxed)',
    lineHeight: 'var(--leading-relaxed)',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    overflowWrap: { default: null, ':is([data-wrap])': 'break-word' },
    whiteSpace: { default: null, ':is([data-wrap])': 'pre-wrap' },
  },
  rule8: {
    position: 'relative',
    marginInline:
      'calc(var(--code-block-padding, calc(var(--spacing) * 3)) * -1)',
    display: {
      default: 'block',
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'grid',
    },
    minHeight: '1lh',
    paddingInline: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    gridColumn: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'span 2 / span 2',
    },
    gridTemplateColumns: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'subgrid',
    },
    columnGap: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        `calc(${themeLayout['--spacing']} * 3)`,
    },
    counterIncrement: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'line',
    },
    '::before': {
      content: {
        default: 'var(--tw-content)',
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'var(--tw-content)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'var(--tw-content)',
        },
      },
      minWidth: `calc(${themeLayout['--spacing']} * 6)`,
      textAlign: 'end',
      color: themeColors['--color-muted-foreground'],
      '--tw-content': {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'counter(line)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'counter(line) / ""',
        },
      },
      WebkitUserSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
      userSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
    },
    '::after': {
      content: 'var(--tw-content)',
      position: 'absolute',
      insetInlineStart: `calc(${themeLayout['--spacing']} * 1)`,
      top: 0,
      color: themeColors['--color-muted-foreground'],
      WebkitUserSelect: 'none',
      userSelect: 'none',
    },
  },
  rule9: {
    position: 'relative',
    marginInline:
      'calc(var(--code-block-padding, calc(var(--spacing) * 3)) * -1)',
    display: {
      default: 'block',
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'grid',
    },
    minHeight: '1lh',
    backgroundColor: themeColors['--color-muted'],
    paddingInline: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    gridColumn: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'span 2 / span 2',
    },
    gridTemplateColumns: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'subgrid',
    },
    columnGap: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        `calc(${themeLayout['--spacing']} * 3)`,
    },
    counterIncrement: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'line',
    },
    '::before': {
      content: {
        default: 'var(--tw-content)',
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'var(--tw-content)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'var(--tw-content)',
        },
      },
      minWidth: `calc(${themeLayout['--spacing']} * 6)`,
      textAlign: 'end',
      color: themeColors['--color-muted-foreground'],
      '--tw-content': {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'counter(line)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'counter(line) / ""',
        },
      },
      WebkitUserSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
      userSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
    },
    '::after': {
      content: 'var(--tw-content)',
      position: 'absolute',
      insetInlineStart: `calc(${themeLayout['--spacing']} * 1)`,
      top: 0,
      color: themeColors['--color-muted-foreground'],
      WebkitUserSelect: 'none',
      userSelect: 'none',
    },
  },
  rule10: {
    position: 'relative',
    marginInline:
      'calc(var(--code-block-padding, calc(var(--spacing) * 3)) * -1)',
    display: {
      default: 'block',
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'grid',
    },
    minHeight: '1lh',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    paddingInline: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    gridColumn: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'span 2 / span 2',
    },
    gridTemplateColumns: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'subgrid',
    },
    columnGap: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        `calc(${themeLayout['--spacing']} * 3)`,
    },
    counterIncrement: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'line',
    },
    '::before': {
      content: {
        default: 'var(--tw-content)',
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'var(--tw-content)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'var(--tw-content)',
        },
      },
      minWidth: `calc(${themeLayout['--spacing']} * 6)`,
      textAlign: 'end',
      color: themeColors['--color-muted-foreground'],
      '--tw-content': {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'counter(line)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'counter(line) / ""',
        },
      },
      WebkitUserSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
      userSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
    },
    '::after': {
      content: 'var(--tw-content)',
      position: 'absolute',
      insetInlineStart: `calc(${themeLayout['--spacing']} * 1)`,
      top: 0,
      color: themeColors['--color-destructive'],
      '--tw-content': '"−"',
      WebkitUserSelect: 'none',
      userSelect: 'none',
    },
  },
  rule11: {
    position: 'relative',
    marginInline:
      'calc(var(--code-block-padding, calc(var(--spacing) * 3)) * -1)',
    display: {
      default: 'block',
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'grid',
    },
    minHeight: '1lh',
    backgroundColor: themeColors['--color-muted'],
    paddingInline: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    gridColumn: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'span 2 / span 2',
    },
    gridTemplateColumns: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        'subgrid',
    },
    columnGap: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
        `calc(${themeLayout['--spacing']} * 3)`,
    },
    counterIncrement: {
      default: null,
      [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'line',
    },
    '::before': {
      content: {
        default: 'var(--tw-content)',
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'var(--tw-content)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'var(--tw-content)',
        },
      },
      minWidth: `calc(${themeLayout['--spacing']} * 6)`,
      textAlign: 'end',
      color: themeColors['--color-muted-foreground'],
      '--tw-content': {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
          'counter(line)',
        "@supports (content: counter(line) / '')": {
          default: null,
          [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]:
            'counter(line) / ""',
        },
      },
      WebkitUserSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
      userSelect: {
        default: null,
        [stylex.when.ancestor(':is([data-numbered])', codeGroupMarker)]: 'none',
      },
    },
    '::after': {
      content: 'var(--tw-content)',
      position: 'absolute',
      insetInlineStart: `calc(${themeLayout['--spacing']} * 1)`,
      top: 0,
      color: themeColors['--color-muted-foreground'],
      '--tw-content': '"+"',
      WebkitUserSelect: 'none',
      userSelect: 'none',
    },
  },
  rule12: {
    display: 'flex',
    minWidth: 0,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    paddingBottom: 'var(--code-block-padding, calc(var(--spacing) * 3))',
    color: themeColors['--color-muted-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule2: {
    display: 'grid',
    minHeight: '1lh',
    minWidth: { default: 'max-content', ':where([data-wrap] *)': 0 },
    alignContent: 'flex-start',
    gridTemplateColumns: {
      default: null,
      ':where([data-numbered] *)': 'auto 1fr',
    },
  },
  rule3: { minWidth: 0 },
})

const codeBlockVariants = (props?: {
  variant?: 'default' | 'destructive' | null
  class?: stylex.StyleXStyles
  className?: stylex.StyleXStyles
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? 'default' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? [stylexStyles2.rule3, codeBlockGroupMarker]
      : variant0 === 'destructive'
        ? [stylexStyles2.rule2, codeBlockGroupMarker]
        : [stylexStyles2.rule1, codeBlockGroupMarker],
    props?.class,
    props?.className,
  ] as const
}

const layoutStyles = stylex.create({
  regular: {
    '--code-block-padding': `calc(${themeLayout['--spacing']} * 3)`,
    '--code-block-action-inset': `calc(${themeLayout['--spacing']} * 2)`,
    '--code-block-title-size': 'var(--text-sm)',
    '--code-block-title-leading': 'var(--text-sm--line-height)',
    '--code-block-content-top': {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      ':has([data-slot="code-block-header"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
    '--code-block-content-bottom': {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      ':has([data-slot="code-block-footer"])': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
  small: {
    '--code-block-padding': `calc(${themeLayout['--spacing']} * 2.5)`,
    '--code-block-action-inset': `calc(${themeLayout['--spacing']} * 1.5)`,
    '--code-block-title-size': 'var(--text-xs)',
    '--code-block-title-leading': 'var(--text-xs--line-height)',
    '--code-block-content-top': {
      default: `calc(${themeLayout['--spacing']} * 2.5)`,
      ':has([data-slot="code-block-header"])': `calc(${themeLayout['--spacing']} * 1.5)`,
    },
    '--code-block-content-bottom': `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  normal: {
    '--code-block-title-color': 'var(--color-foreground)',
    '--code-block-header-color': 'var(--color-muted-foreground)',
  },
  destructive: {
    '--code-block-title-color': 'var(--color-destructive)',
    '--code-block-header-color': 'var(--color-destructive)',
  },
})

/** CodeBlock scopes compact spacing and semantic tone to its content parts. */
function CodeBlock({
  sx,
  variant = 'default',
  size = 'default',
  ...props
}: Omit<
  React.ComponentProps<'div'> &
    VariantProps<typeof codeBlockVariants> & { size?: 'default' | 'sm' },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <div
      data-slot="code-block"
      data-variant={variant}
      data-size={size}
      {...stylexProps(
        [
          codeBlockVariants({ variant: variant }),
          size === 'sm' ? layoutStyles.small : layoutStyles.regular,
          variant === 'destructive'
            ? layoutStyles.destructive
            : layoutStyles.normal,
          sx,
        ],
        props,
      )}
    />
  )
}

/** CodeBlockHeader aligns the title and inline actions above the content. */
function CodeBlockHeader({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="code-block-header"
      {...stylexProps([stylexStyles2.rule4, sx], props)}
    />
  )
}

/** CodeBlockTitle keeps title text and inherited icons inside the available width. */
function CodeBlockTitle({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <IconStylesProvider
      sx={[iconDefaults.codeTitle]}
      requiredSx={iconRequired.codeTitle}
    >
      <div
        data-slot="code-block-title"
        {...stylexProps([codeTitleIconMarker, stylexStyles2.rule5, sx], props)}
      />
    </IconStylesProvider>
  )
}

const controlStyles = stylex.create({
  codeAction: {
    backgroundColor: {
      default: null,
      ':is(button)': {
        default: null,
        [stylex.when.ancestor(
          ':is(*) > :is([data-slot="code-block-actions"]) >',
          codeBlockGroupMarker,
        )]: themeColors['--color-background'],
        ':hover': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]: themeColors['--color-background'],
        },
        ':focus': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]: themeColors['--color-background'],
        },
        ':focus-visible': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]: themeColors['--color-background'],
        },
        ':active': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]: themeColors['--color-background'],
        },
        '@media (hover: hover)': {
          default: null,
          ':hover': {
            default: null,
            [stylex.when.ancestor(
              ':is(*) > :is([data-slot="code-block-actions"]) >',
              codeBlockGroupMarker,
            )]: themeColors['--color-background'],
          },
        },
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':is(button)': {
        default: null,
        [stylex.when.ancestor(
          ':is(*) > :is([data-slot="code-block-actions"]) >',
          codeBlockGroupMarker,
        )]:
          'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
        ':hover': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
        },
        ':focus': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
        },
        ':focus-visible': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
        },
        ':active': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
        },
        '@media (hover: hover)': {
          default: null,
          ':hover': {
            default: null,
            [stylex.when.ancestor(
              ':is(*) > :is([data-slot="code-block-actions"]) >',
              codeBlockGroupMarker,
            )]:
              'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
          },
        },
      },
    },
    '--tw-ring-color': {
      default: null,
      ':is(button)': {
        default: null,
        [stylex.when.ancestor(
          ':is(*) > :is([data-slot="code-block-actions"]) >',
          codeBlockGroupMarker,
        )]:
          `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
        ':hover': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
        },
        ':focus': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
        },
        ':focus-visible': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
        },
        ':active': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
        },
        '@media (hover: hover)': {
          default: null,
          ':hover': {
            default: null,
            [stylex.when.ancestor(
              ':is(*) > :is([data-slot="code-block-actions"]) >',
              codeBlockGroupMarker,
            )]:
              `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
          },
        },
      },
    },
    boxShadow: {
      default: null,
      ':is(button)': {
        default: null,
        [stylex.when.ancestor(
          ':is(*) > :is([data-slot="code-block-actions"]) >',
          codeBlockGroupMarker,
        )]:
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
        ':hover': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
        },
        ':focus': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
        },
        ':focus-visible': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
        },
        ':active': {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > :is([data-slot="code-block-actions"]) >',
            codeBlockGroupMarker,
          )]:
            'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
        },
        '@media (hover: hover)': {
          default: null,
          ':hover': {
            default: null,
            [stylex.when.ancestor(
              ':is(*) > :is([data-slot="code-block-actions"]) >',
              codeBlockGroupMarker,
            )]:
              'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
          },
        },
      },
    },
  },
})

/** CodeBlockActions overlays controls when it is a direct child of the block. */
function CodeBlockActions({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <ControlStylesProvider styles={controlStyles.codeAction}>
      <div
        data-slot="code-block-actions"
        {...stylexProps([stylexStyles2.rule6, sx], props)}
      />
    </ControlStylesProvider>
  )
}

/** CodeBlockCopyButton copies its value and announces the completed write. */
function CodeBlockCopyButton({
  value,
  variant = 'ghost',
  size = 'iconXs',
  sx,
  onClick,
  ...props
}: React.ComponentProps<typeof Button> & { value: string }) {
  const [copied, setCopied] = React.useState(false)

  React.useEffect(() => {
    if (!copied) {
      return
    }

    const timeout = setTimeout(() => setCopied(false), 2000)
    return () => clearTimeout(timeout)
  }, [copied])

  return (
    <>
      <Button
        data-slot="code-block-copy-button"
        data-copied={copied || undefined}
        variant={variant}
        size={size}
        aria-label={copied ? 'Copied' : 'Copy'}
        sx={sx}
        onClick={(event) => {
          onClick?.(event)
          void navigator.clipboard.writeText(value).then(
            () => setCopied(true),
            () => setCopied(false),
          )
        }}
        {...props}
      >
        {copied ? <LuCheck /> : <LuCopy />}
      </Button>
      <span aria-live="polite" sx={stylexStyles.rule1}>
        {copied ? 'Copied' : ''}
      </span>
    </>
  )
}

// numberedCodeLines splits a source string into lines. Repeated lines keep
// their own key so a reorder does not reuse the wrong row.
function numberedCodeLines(value: string) {
  const seen = new Map<string, number>()

  return value
    .replace(/\n$/, '')
    .split('\n')
    .map((line) => {
      const count = seen.get(line) ?? 0
      seen.set(line, count + 1)
      return <CodeBlockLine key={`${count}:${line}`}>{line}</CodeBlockLine>
    })
}

/** CodeBlockContent preserves caller styles and exposes its line-counter start variable. */
function CodeBlockContent({
  sx,
  children,
  style,
  wrap = false,
  lineNumbers = false,
  startLine = 1,
  ...props
}: Omit<
  React.ComponentProps<'pre'> & {
    wrap?: boolean
    lineNumbers?: boolean
    startLine?: number
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <pre
      data-slot="code-block-content"
      data-wrap={wrap || undefined}
      data-numbered={lineNumbers || undefined}
      dir="ltr"
      tabIndex={0}
      {...stylexProps([codeGroupMarker, stylexStyles2.rule7, sx], {
        ...props,
        style: { ...style, '--code-block-start': startLine - 1 },
      })}
    >
      <code data-slot="code-block-code" sx={stylexStyles.rule2}>
        {lineNumbers && typeof children === 'string'
          ? numberedCodeLines(children)
          : children}
      </code>
    </pre>
  )
}

const codeBlockLineVariants = (props?: {
  variant?: 'default' | 'added' | 'removed' | 'highlighted' | null
  class?: stylex.StyleXStyles
  className?: stylex.StyleXStyles
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? 'default' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? stylexStyles2.rule8
      : variant0 === 'added'
        ? stylexStyles2.rule11
        : variant0 === 'removed'
          ? stylexStyles2.rule10
          : variant0 === 'highlighted'
            ? stylexStyles2.rule9
            : stylexStyles2.rule8,
    props?.class,
    props?.className,
  ] as const
}

/** CodeBlockLine retains empty lines so selection keeps the source layout. */
function CodeBlockLine({
  sx,
  variant = 'default',
  children,
  ...props
}: Omit<
  React.ComponentProps<'span'> & VariantProps<typeof codeBlockLineVariants>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  // An empty line holds a newline so copying a selection keeps blank lines.
  const empty =
    children === undefined ||
    children === null ||
    children === '' ||
    (Array.isArray(children) && children.length === 0)

  return (
    <span
      data-slot="code-block-line"
      data-variant={variant}
      {...stylexProps([codeBlockLineVariants({ variant: variant }), sx], props)}
    >
      <span data-slot="code-block-line-content" sx={stylexStyles.rule3}>
        {empty ? '\n' : children}
      </span>
    </span>
  )
}

/** CodeBlockFooter aligns metadata beneath the content with the block's spacing. */
function CodeBlockFooter({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="code-block-footer"
      {...stylexProps([stylexStyles2.rule12, sx], props)}
    />
  )
}

export {
  CodeBlock,
  CodeBlockHeader,
  CodeBlockTitle,
  CodeBlockActions,
  CodeBlockCopyButton,
  CodeBlockContent,
  CodeBlockLine,
  CodeBlockFooter,
}
