import { keyframeNames } from '../style/tokens.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { LuX } from '@s4wave/web/images/icons.js'

import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'

import * as stylex from '@stylexjs/stylex'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  motion as themeMotion,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
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
  closeIcon: { flexShrink: 0, pointerEvents: 'none', height: 16, width: 16 },
  contentBase: {
    '--tw-duration': '.2s',
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-enter-scale': { default: null, ':is([data-state="open"])': '.95' },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    '--tw-translate-x': '-50%',
    '--tw-translate-y': '-50%',
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
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    position: 'fixed',
    transitionDuration: '.2s',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    zIndex: 50,
    left: '50%',
    top: '50%',
  },
  headerBase: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  overlay: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
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
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: 'color-mix(in oklab, var(--color-black) 50%, transparent)',
    position: 'fixed',
    zIndex: 50,
  },
  contentCompact: {
    padding: 0,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: 0,
    backgroundColor: themeColors['--color-background'],
    display: 'grid',
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'var(--container-lg)',
    },
    width: '100%',
  },
  contentEditor: {
    padding: 0,
    borderRadius: {
      default: 0,
      '@media (min-width: 40rem)': themeRadii['--radius-lg'],
    },
    borderStyle: {
      default: themeUtilityState['--tw-border-style'],
      '@media (min-width: 40rem)': themeUtilityState['--tw-border-style'],
    },
    borderWidth: { default: 0, '@media (min-width: 40rem)': '1px' },
    gap: 0,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-card'],
    display: 'flex',
    flexDirection: 'column',
    height: {
      default: '100dvh',
      '@media (min-width: 40rem)': themeLayout['--height-keybinding-editor'],
    },
    maxWidth: {
      default: 'none',
      '@media (min-width: 40rem)': 'var(--container-5xl)',
    },
    width: {
      default: '100vw',
      '@media (min-width: 40rem)': themeLayout['--width-command-editor'],
    },
  },
  contentPanel: {
    padding: 0,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-get-started'],
    display: 'grid',
    maxWidth: {
      default: 'var(--container-md)',
      '@media (min-width: 40rem)': 'var(--container-lg)',
    },
    width: '100%',
  },
  contentDefault: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: themeColors['--color-background'],
    display: 'grid',
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'var(--container-lg)',
    },
    width: '100%',
  },
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
      ':is([data-state="open"])': themeColors['--color-accent'],
    },
    boxShadow: {
      default: null,
      ':focus':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-muted-foreground'],
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
  headerEditor: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    flexShrink: 0,
    textAlign: { default: 'left', '@media (min-width: 40rem)': 'left' },
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    paddingRight: {
      default: `calc(${themeLayout['--spacing']} * 12)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
  },
  headerPanel: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: { default: 'left', '@media (min-width: 40rem)': 'left' },
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  headerDefault: {
    textAlign: { default: 'center', '@media (min-width: 40rem)': 'left' },
  },
  footer: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: {
      default: 'column-reverse',
      '@media (min-width: 40rem)': 'row',
    },
    justifyContent: { default: null, '@media (min-width: 40rem)': 'flex-end' },
  },
  title: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-leading': '1',
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 1,
  },
  withIcon: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  editorTitle: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    // The complete editor recipe removes the default title's leading-none variable.
    '--tw-leading': null,
    '--tw-tracking': 'var(--tracking-tight)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: 'var(--text-base)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
  },
  description: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  editorDescription: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  relaxedDescription: {
    '--tw-leading': 'var(--leading-relaxed)',
    lineHeight: 'var(--leading-relaxed)',
  },
})

/** Dialog provides the open state shared by its dialog controls. */
function Dialog({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

/** DialogTrigger opens its dialog. */
function DialogTrigger({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof DialogPrimitive.Trigger>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <DialogPrimitive.Trigger
      data-slot="dialog-trigger"
      {...stylexProps(sx, props)}
    />
  )
}

/** DialogPortal renders dialog content outside clipped ancestors. */
function DialogPortal({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

/** DialogClose closes its dialog. */
function DialogClose({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof DialogPrimitive.Close>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <DialogPrimitive.Close
      data-slot="dialog-close"
      {...stylexProps(sx, props)}
    />
  )
}

/** DialogOverlay dims the area behind a modal dialog. */
function DialogOverlay({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof DialogPrimitive.Overlay>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      {...stylexProps([styles.overlay, sx], props)}
    />
  )
}

/** DialogContent renders a responsive dialog panel with an optional close control. */
function DialogContent({
  sx,
  children,
  showCloseButton = true,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof DialogPrimitive.Content> & {
    showCloseButton?: boolean
    variant?: 'default' | 'compact' | 'editor' | 'panel'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const environment = useAppEnvironment()
  return (
    <DialogPortal data-slot="dialog-portal">
      <DialogOverlay />
      <DialogPrimitive.Content
        data-spacewave-app={environment.id || undefined}
        data-slot="dialog-content"
        {...stylexProps(
          [
            styles.contentBase,
            variant === 'compact'
              ? styles.contentCompact
              : variant === 'editor'
                ? styles.contentEditor
                : variant === 'panel'
                  ? styles.contentPanel
                  : styles.contentDefault,
            sx,
          ],
          props,
        )}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            {...stylex.props(styles.close)}
          >
            <LuX sx={styles.closeIcon} />
            <span sx={styles.closeLabel}>Close</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  )
}

/** DialogHeader lays out the dialog heading. */
function DialogHeader({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    variant?: 'default' | 'editor' | 'panel'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <div
      data-slot="dialog-header"
      {...stylexProps(
        [
          styles.headerBase,
          variant === 'editor'
            ? styles.headerEditor
            : variant === 'panel'
              ? styles.headerPanel
              : styles.headerDefault,
          sx,
        ],
        props,
      )}
    />
  )
}

/** DialogFooter lays out the dialog actions. */
function DialogFooter({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="dialog-footer"
      {...stylexProps([styles.footer, sx], props)}
    />
  )
}

/** DialogTitle names the dialog for assistive technology. */
function DialogTitle({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof DialogPrimitive.Title> & {
    variant?: 'default' | 'withIcon' | 'editor'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      {...stylexProps(
        [
          styles.title,
          variant === 'withIcon' && styles.withIcon,
          variant === 'editor' && styles.editorTitle,
          sx,
        ],
        props,
      )}
    />
  )
}

/** DialogDescription describes the dialog for assistive technology. */
function DialogDescription({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof DialogPrimitive.Description> & {
    variant?: 'default' | 'editor' | 'relaxed'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      {...stylexProps(
        [
          styles.description,
          variant === 'editor' && styles.editorDescription,
          variant === 'relaxed' && styles.relaxedDescription,
          sx,
        ],
        props,
      )}
    />
  )
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
}
