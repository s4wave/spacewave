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
    position: 'fixed',
    inset: 0,
    zIndex: 50,
    backgroundColor: 'color-mix(in oklab, var(--color-black) 50%, transparent)',
    animationName: {
      default: null,
      ':is([data-state="closed"])': 'shadcn-exit',
      ':is([data-state="open"])': 'shadcn-enter',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
  },
  content: {
    zIndex: 50,
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: themeColors['--color-background'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: 'var(--ease-in-out)',
    transitionDuration: {
      default: 'var(--tw-duration, var(--default-transition-duration))',
      ':is([data-state="closed"])': '.3s',
      ':is([data-state="open"])': '.5s',
    },
    '--tw-ease': 'var(--ease-in-out)',
    animationName: {
      default: null,
      ':is([data-state="closed"])': 'shadcn-exit',
      ':is([data-state="open"])': 'shadcn-enter',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    '--tw-duration': {
      default: null,
      ':is([data-state="closed"])': '.3s',
      ':is([data-state="open"])': '.5s',
    },
  },
  absolute: { position: 'absolute' },
  fixed: { position: 'fixed' },
  right: {
    insetBlock: 0,
    right: 0,
    height: '100%',
    width: '75%',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    '--tw-exit-translate-x': {
      default: null,
      ':is([data-state="closed"])': '100%',
    },
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-state="open"])': '100%',
    },
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': 'var(--container-sm)',
    },
  },
  left: {
    insetBlock: 0,
    left: 0,
    height: '100%',
    width: '75%',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    '--tw-exit-translate-x': {
      default: null,
      ':is([data-state="closed"])': '-100%',
    },
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-state="open"])': '-100%',
    },
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': 'var(--container-sm)',
    },
  },
  top: {
    insetInline: 0,
    top: 0,
    height: 'auto',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    '--tw-exit-translate-y': {
      default: null,
      ':is([data-state="closed"])': '-100%',
    },
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-state="open"])': '-100%',
    },
  },
  bottom: {
    insetInline: 0,
    bottom: 0,
    height: 'auto',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    '--tw-exit-translate-y': {
      default: null,
      ':is([data-state="closed"])': '100%',
    },
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-state="open"])': '100%',
    },
  },
  docs: { gap: 0, padding: 0 },
  header: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  footer: {
    marginTop: 'auto',
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  title: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  docsTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  plainTitle: { gap: 0 },
  description: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  absoluteOverlay: { position: 'absolute', inset: 0 },
  close: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 4)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: 'var(--radius-xs)',
    opacity: {
      default: 0.7,
      '@media (hover: hover)': { default: null, ':hover': 1 },
    },
    '--tw-ring-offset-color': themeColors['--color-background'],
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': { default: null, ':focus': themeColors['--color-ring'] },
    '--tw-ring-offset-width': { default: null, ':focus': '2px' },
    '--tw-ring-offset-shadow': {
      default: null,
      ':focus':
        'var(--tw-ring-inset, ) 0 0 0 var(--tw-ring-offset-width) var(--tw-ring-offset-color)',
    },
    '--tw-outline-style': { default: null, ':focus': 'none' },
    outlineStyle: { default: null, ':focus': 'none' },
    outlineOffset: {
      default: null,
      '@media (forced-colors: active)': { default: null, ':focus': '2px' },
    },
    outline: {
      default: null,
      '@media (forced-colors: active)': {
        default: null,
        ':focus': '2px solid #0000',
      },
    },
    pointerEvents: { default: null, ':disabled': 'none' },
    backgroundColor: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-secondary'],
    },
  },
  closeIcon: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  closeLabel: {
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
      {...stylex.props(sx)}
      {...props}
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
    <SheetPrimitive.Close
      data-slot="sheet-close"
      {...stylex.props(sx)}
      {...props}
    />
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
      {...stylex.props([styles.overlay, sx])}
      {...props}
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
        {...stylex.props([
          styles.content,
          position === 'absolute' ? styles.absolute : styles.fixed,
          sideStyles[side],
          (variant === 'docs' || variant === 'docsPlain') && styles.docs,
          sx,
        ])}
        {...props}
      >
        {children}
        {showCloseButton && (
          <SheetPrimitive.Close {...stylex.props(styles.close)}>
            <XIcon {...stylex.props(styles.closeIcon)} />
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
  return <div data-slot="sheet-header" sx={[styles.header, sx]} {...props} />
}

/** SheetFooter lays out the final sheet actions. */
function SheetFooter({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: StyleXStyles
  className?: never
}) {
  return <div data-slot="sheet-footer" sx={[styles.footer, sx]} {...props} />
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
      {...stylex.props([
        styles.title,
        (variant === 'docs' || variant === 'docsPlain') && styles.docsTitle,
        variant === 'docsPlain' && styles.plainTitle,
        sx,
      ])}
      {...props}
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
      {...stylex.props([styles.description, sx])}
      {...props}
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
