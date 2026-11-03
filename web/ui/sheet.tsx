import { keyframeNames } from '../style/tokens.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as SheetPrimitive from '@radix-ui/react-dialog'
import { LuX as XIcon } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  overlay: {
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    inset: 0,
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationName: {
      default: null,
      ':is([data-state="closed"])': keyframeNames['shadcn-exit'],
      ':is([data-state="open"])': keyframeNames['shadcn-enter'],
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    backgroundColor: 'color-mix(in oklab, var(--color-black) 50%, transparent)',
    position: 'fixed',
    zIndex: 50,
  },
  content: {
    '--tw-duration': {
      default: null,
      ':is([data-state="closed"])': '.3s',
      ':is([data-state="open"])': '.5s',
    },
    '--tw-ease': 'var(--ease-in-out)',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationName: {
      default: null,
      ':is([data-state="closed"])': keyframeNames['shadcn-exit'],
      ':is([data-state="open"])': keyframeNames['shadcn-enter'],
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    backgroundColor: themeColors['--color-background'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    flexDirection: 'column',
    transitionDuration: {
      default: 'var(--tw-duration, var(--default-transition-duration))',
      ':is([data-state="closed"])': '.3s',
      ':is([data-state="open"])': '.5s',
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: 'var(--ease-in-out)',
    zIndex: 50,
  },
  absolute: { position: 'absolute' },
  fixed: { position: 'fixed' },
  right: {
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-state="open"])': '100%',
    },
    '--tw-exit-translate-x': {
      default: null,
      ':is([data-state="closed"])': '100%',
    },
    insetBlock: 0,
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    height: '100%',
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': 'var(--container-sm)',
    },
    right: 0,
    width: '75%',
  },
  left: {
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-state="open"])': '-100%',
    },
    '--tw-exit-translate-x': {
      default: null,
      ':is([data-state="closed"])': '-100%',
    },
    insetBlock: 0,
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    height: '100%',
    left: 0,
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': 'var(--container-sm)',
    },
    width: '75%',
  },
  top: {
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-state="open"])': '-100%',
    },
    '--tw-exit-translate-y': {
      default: null,
      ':is([data-state="closed"])': '-100%',
    },
    insetInline: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: 'auto',
    top: 0,
  },
  bottom: {
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-state="open"])': '100%',
    },
    '--tw-exit-translate-y': {
      default: null,
      ':is([data-state="closed"])': '100%',
    },
    insetInline: 0,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    bottom: 0,
    height: 'auto',
  },
  docs: { padding: 0, gap: 0 },
  header: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    flexDirection: 'column',
  },
  footer: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    marginTop: 'auto',
  },
  title: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontWeight: 'var(--font-weight-semibold)',
  },
  docsTitle: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  plainTitle: { gap: 0 },
  description: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  absoluteOverlay: { inset: 0, position: 'absolute' },
  close: {
    '--tw-outline-style': { default: null, ':focus': 'none' },
    '--tw-ring-color': { default: null, ':focus': themeColors['--color-ring'] },
    '--tw-ring-offset-color': themeColors['--color-background'],
    '--tw-ring-offset-shadow': {
      default: null,
      ':focus':
        'var(--tw-ring-inset, ) 0 0 0 var(--tw-ring-offset-width) var(--tw-ring-offset-color)',
    },
    '--tw-ring-offset-width': { default: null, ':focus': '2px' },
    '--tw-ring-shadow': {
      default: null,
      ':focus':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: 'var(--radius-xs)',
    backgroundColor: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-secondary'],
    },
    boxShadow: {
      default: null,
      ':focus':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    opacity: {
      default: 0.7,
      '@media (hover: hover)': { default: null, ':hover': 1 },
    },
    outlineColor: {
      default: null,
      '@media (forced-colors: active)': { default: null, ':focus': '#0000' },
    },
    outlineOffset: {
      default: null,
      '@media (forced-colors: active)': { default: null, ':focus': '2px' },
    },
    outlineStyle: {
      default: null,
      ':focus': 'none',
      '@media (forced-colors: active)': { default: null, ':focus': 'solid' },
    },
    outlineWidth: {
      default: null,
      '@media (forced-colors: active)': { default: null, ':focus': '2px' },
    },
    pointerEvents: { default: null, ':disabled': 'none' },
    position: 'absolute',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * 4)`,
  },
  closeIcon: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  closeLabel: {
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
  },
})

const sideStyles = {
  right: styles.right,
  left: styles.left,
  top: styles.top,
  bottom: styles.bottom,
}

/** Sheet provides the open state shared by its trigger and panel. */
function Sheet({ ...props }: React.ComponentProps<typeof SheetPrimitive.Root>) {
  return <SheetPrimitive.Root data-slot="sheet" {...props} />
}

/** SheetTrigger opens its sheet. */
function SheetTrigger({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof SheetPrimitive.Trigger>, 'className'> & {
  sx?: StyleXStyles
  className?: never
}) {
  return (
    <SheetPrimitive.Trigger
      data-slot="sheet-trigger"
      {...stylexProps(sx, props)}
    />
  )
}

/** SheetClose closes its sheet. */
function SheetClose({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof SheetPrimitive.Close>, 'className'> & {
  sx?: StyleXStyles
  className?: never
}) {
  return (
    <SheetPrimitive.Close data-slot="sheet-close" {...stylexProps(sx, props)} />
  )
}

/** SheetPortal renders the panel in its chosen container. */
function SheetPortal({
  ...props
}: React.ComponentProps<typeof SheetPrimitive.Portal>) {
  return <SheetPrimitive.Portal data-slot="sheet-portal" {...props} />
}

/** SheetOverlay dims the area behind an open modal sheet. */
function SheetOverlay({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof SheetPrimitive.Overlay>, 'className'> & {
  sx?: StyleXStyles
  className?: never
}) {
  return (
    <SheetPrimitive.Overlay
      data-slot="sheet-overlay"
      {...stylexProps([styles.overlay, sx], props)}
    />
  )
}

/** SheetContent positions the panel and renders its optional close action. */
function SheetContent({
  sx,
  children,
  portalContainer,
  position = 'fixed',
  showCloseButton = true,
  side = 'right',
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof SheetPrimitive.Content> & {
    portalContainer?: HTMLElement | null
    position?: 'fixed' | 'absolute'
    showCloseButton?: boolean
    side?: 'top' | 'right' | 'bottom' | 'left'
    variant?: 'default' | 'docs' | 'docsPlain'
  },
  'className'
> & { sx?: StyleXStyles; className?: never }) {
  return (
    <SheetPortal container={portalContainer}>
      <SheetOverlay
        sx={position === 'absolute' ? styles.absoluteOverlay : undefined}
      />
      <SheetPrimitive.Content
        data-slot="sheet-content"
        {...stylexProps(
          [
            styles.content,
            position === 'absolute' ? styles.absolute : styles.fixed,
            sideStyles[side],
            (variant === 'docs' || variant === 'docsPlain') && styles.docs,
            sx,
          ],
          props,
        )}
      >
        {children}
        {showCloseButton && (
          <SheetPrimitive.Close {...stylex.props(styles.close)}>
            <XIcon sx={styles.closeIcon} />
            <span sx={styles.closeLabel}>Close</span>
          </SheetPrimitive.Close>
        )}
      </SheetPrimitive.Content>
    </SheetPortal>
  )
}

/** SheetHeader lays out the sheet heading. */
function SheetHeader({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: StyleXStyles
  className?: never
}) {
  return (
    <div
      data-slot="sheet-header"
      {...stylexProps([styles.header, sx], props)}
    />
  )
}

/** SheetFooter lays out the final sheet actions. */
function SheetFooter({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: StyleXStyles
  className?: never
}) {
  return (
    <div
      data-slot="sheet-footer"
      {...stylexProps([styles.footer, sx], props)}
    />
  )
}

/** SheetTitle labels the dialog for assistive technology. */
function SheetTitle({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof SheetPrimitive.Title> & {
    variant?: 'default' | 'docs' | 'docsPlain'
  },
  'className'
> & { sx?: StyleXStyles; className?: never }) {
  return (
    <SheetPrimitive.Title
      data-slot="sheet-title"
      {...stylexProps(
        [
          styles.title,
          (variant === 'docs' || variant === 'docsPlain') && styles.docsTitle,
          variant === 'docsPlain' && styles.plainTitle,
          sx,
        ],
        props,
      )}
    />
  )
}

/** SheetDescription supplies the dialog description. */
function SheetDescription({
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof SheetPrimitive.Description>,
  'className'
> & { sx?: StyleXStyles; className?: never }) {
  return (
    <SheetPrimitive.Description
      data-slot="sheet-description"
      {...stylexProps([styles.description, sx], props)}
    />
  )
}

export {
  Sheet,
  SheetTrigger,
  SheetClose,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
}
