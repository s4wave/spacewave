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
  closeIcon: { width: 16, height: 16, pointerEvents: 'none', flexShrink: 0 },
  contentBase: {
    position: 'fixed',
    top: '50%',
    left: '50%',
    zIndex: 50,
    '--tw-translate-x': '-50%',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    '--tw-translate-y': '-50%',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-duration': '.2s',
    transitionDuration: '.2s',
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
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-enter-scale': { default: null, ':is([data-state="open"])': '.95' },
  },
  headerBase: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  overlay: {
    position: 'fixed',
    inset: 0,
    zIndex: 50,
    backgroundColor: 'color-mix(in oklab, var(--color-black) 50%, transparent)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
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
  contentCompact: {
    display: 'grid',
    width: '100%',
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'var(--container-lg)',
    },
    gap: 0,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background'],
    padding: 0,
  },
  contentEditor: {
    display: 'flex',
    height: {
      default: '100dvh',
      '@media (min-width: 40rem)': themeLayout['--height-keybinding-editor'],
    },
    width: {
      default: '100vw',
      '@media (min-width: 40rem)': themeLayout['--width-command-editor'],
    },
    maxWidth: {
      default: 'none',
      '@media (min-width: 40rem)': 'var(--container-5xl)',
    },
    flexDirection: 'column',
    gap: 0,
    overflow: 'hidden',
    borderRadius: {
      default: 0,
      '@media (min-width: 40rem)': themeRadii['--radius-lg'],
    },
    borderStyle: {
      default: themeUtilityState['--tw-border-style'],
      '@media (min-width: 40rem)': themeUtilityState['--tw-border-style'],
    },
    borderWidth: { default: 0, '@media (min-width: 40rem)': '1px' },
    backgroundColor: themeColors['--color-background-card'],
    padding: 0,
  },
  contentPanel: {
    display: 'grid',
    width: '100%',
    maxWidth: {
      default: 'var(--container-md)',
      '@media (min-width: 40rem)': 'var(--container-lg)',
    },
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    padding: 0,
  },
  contentDefault: {
    display: 'grid',
    width: '100%',
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'var(--container-lg)',
    },
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
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
    outlineStyle: {
      default: null,
      ':focus': 'none',
      '@media (forced-colors: active)': { default: null, ':focus': 'solid' },
    },
    outlineOffset: {
      default: null,
      '@media (forced-colors: active)': { default: null, ':focus': '2px' },
    },
    outlineWidth: {
      default: null,
      '@media (forced-colors: active)': { default: null, ':focus': '2px' },
    },
    outlineColor: {
      default: null,
      '@media (forced-colors: active)': { default: null, ':focus': '#0000' },
    },
    pointerEvents: { default: null, ':disabled': 'none' },
    backgroundColor: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-accent'],
    },
    color: {
      default: null,
      ':is([data-state="open"])': themeColors['--color-muted-foreground'],
    },
  },
  headerEditor: {
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    paddingRight: {
      default: `calc(${themeLayout['--spacing']} * 12)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    textAlign: { default: 'left', '@media (min-width: 40rem)': 'left' },
  },
  headerPanel: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: { default: 'left', '@media (min-width: 40rem)': 'left' },
  },
  headerDefault: {
    textAlign: { default: 'center', '@media (min-width: 40rem)': 'left' },
  },
  footer: {
    display: 'flex',
    flexDirection: {
      default: 'column-reverse',
      '@media (min-width: 40rem)': 'row',
    },
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    justifyContent: { default: null, '@media (min-width: 40rem)': 'flex-end' },
  },
  title: {
    fontSize: 'var(--text-lg)',
    lineHeight: 1,
    '--tw-leading': '1',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  withIcon: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  editorTitle: {
    // The complete editor recipe removes the default title's leading-none variable.
    '--tw-leading': null,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  description: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  editorDescription: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
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
