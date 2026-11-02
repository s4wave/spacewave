import * as iconStylex from '@stylexjs/stylex'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  iconDefaults,
  commandIconMarker,
} from '../style/icon-defaults.stylex.js'
import * as ownerStylex from '@stylexjs/stylex'
import { cn as ownerClasses } from '@s4wave/web/style/utils.js'
import { groupMarker as ownergroupMarker } from '@s4wave/web/style/relations.stylex.js'
import * as React from 'react'
import { Command as CommandPrimitive } from 'cmdk'
import { LuSearch } from '@s4wave/web/images/icons.js'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const commandDialogStyles = stylex.create({
  palette: {
    top: 'auto',
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    maxHeight: themeLayout['--max-height-command-palette'],
    width: themeLayout['--width-command-editor'],
    '--tw-translate-y': '0px',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    overflow: 'hidden',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    padding: 0,
    '--tw-shadow': '0 0 #0000',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'none',
    },
  },
  default: {
    top: '38%',
    maxHeight: 'min(28rem, 100vh - 6rem)',
    overflow: 'hidden',
    padding: 0,
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'var(--container-xl)',
    },
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
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule4: {
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-muted']} 50%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '10px',
    lineHeight: 1,
    '--tw-leading': '1',
  },
})

const commandVariants = cva('', {
  variants: {
    variant: {
      default: 'rounded-md',
      landing:
        'border-foreground/11 bg-background-get-started rounded-landing-launcher shadow-landing-launcher relative min-h-50 border backdrop-blur-sm',
      dashboard:
        'border-ui-outline bg-background-get-started/95 relative max-h-(--max-height-dashboard-list) rounded-lg border shadow-xl backdrop-blur-sm',
      folder: 'rounded-md bg-transparent',
      palette:
        'border-foreground/10 bg-background-card/95 bottom-4 top-auto translate-y-0 overflow-hidden rounded-md shadow-none sm:max-w-none',
    },
  },
  defaultVariants: { variant: 'default' },
})

const commandInputWrapperVariants = cva('', {
  variants: {
    variant: {
      default: '',
      landing: 'border-foreground/7 h-12 px-4.5',
      dashboard: '',
      folder: '',
    },
  },
})

const commandInputVariants = cva('', {
  variants: {
    variant: {
      default: 'text-sm',
      landing: 'placeholder:text-foreground/70 text-landing-prompt',
      dashboard:
        'border-ui-outline placeholder:text-foreground-alt/50 h-11 border-b text-sm',
      folder: 'border-0 text-sm',
    },
  },
  defaultVariants: { variant: 'default' },
})

const commandListVariants = cva('', {
  variants: {
    variant: {
      default: '',
      landing: 'bg-background-get-started pb-2.5',
      transparent: 'bg-transparent',
      palette: 'pb-0',
    },
  },
})

const commandEmptyVariants = cva('', {
  variants: {
    variant: {
      default: '',
      dashboard: 'text-foreground-alt py-8 text-center text-sm',
      folder: 'text-foreground-alt/40 px-3 py-2 text-left text-xs',
    },
  },
})

// groupHeadingCaps sets group headings as small, spaced capitals. The
// landing launcher instead matches its prerendered sentence-case labels, so
// the headings do not change when the page hydrates.
const groupHeadingCaps =
  '[&_[cmdk-group-heading]]:text-foreground-alt/70 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:micro-ten [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-widest [&_[cmdk-group-heading]]:uppercase'

const commandGroupVariants = cva('', {
  variants: {
    variant: {
      default: cn(groupHeadingCaps, '[&_[cmdk-group-heading]]:px-2'),
      compact: cn(groupHeadingCaps, 'py-1 [&_[cmdk-group-heading]]:px-2'),
      palette: cn(groupHeadingCaps, '!px-0 [&_[cmdk-group-heading]]:px-3'),
      landing:
        '[&_[cmdk-group-heading]]:text-foreground/50 py-0 [&_[cmdk-group-heading]]:px-4 [&_[cmdk-group-heading]]:pt-2.5 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:tracking-wide',
    },
  },
  defaultVariants: { variant: 'default' },
})

const commandItemVariants = cva('', {
  variants: {
    variant: {
      default: '',
      landing:
        'text-foreground-alt mx-1 flex rounded-lg cursor-pointer items-center gap-3 px-3 py-1.5 duration-200',
      dashboard: ownerClasses(
        'group flex cursor-pointer items-center gap-3 rounded-md bg-transparent px-3 py-1.5',
        ownerStylex.props(ownergroupMarker).className,
      ),
      dashboardIdentifier: ownerClasses(
        'group flex cursor-pointer items-center gap-3 rounded-md bg-transparent px-3 py-1.5 pr-14 fine-pointer:pr-11',
        ownerStylex.props(ownergroupMarker).className,
      ),
      palette:
        'min-h-12 rounded-none border-b border-foreground/6 px-3 py-2 data-[selected=true]:bg-brand/25',
      paletteDisabled:
        'min-h-12 rounded-none border-b border-foreground/6 px-3 py-2 data-[selected=true]:bg-brand/25 opacity-50',
      back: 'text-foreground-alt',
      conflict: 'text-warning',
      compact: 'flex items-center gap-2 text-xs',
    },
  },
})

const commandShortcutVariants = cva('', {
  variants: {
    variant: {
      default: '',
      brand: 'text-brand/90 shrink-0 pl-4',
      warning: 'text-warning',
    },
  },
})

function Command({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof CommandPrimitive> &
  VariantProps<typeof commandVariants>) {
  return (
    <CommandPrimitive
      data-slot="command"
      className={
        /* TODO(stylex): computed class helper */ cn(
          'bg-popover text-popover-foreground flex max-h-[inherit] w-full flex-col overflow-hidden',
          commandVariants({ variant }),
          className,
        )
      }
      {...props}
    />
  )
}

/** CommandDialog renders the command surface in a modal dialog. */
function CommandDialog({
  title = 'Command Palette',
  description = 'Search for a command to run...',
  children,
  sx,
  showCloseButton = true,
  variant,
  ...props
}: React.ComponentProps<typeof Dialog> & {
  title?: string
  description?: string
  sx?: stylex.StyleXStyles
  className?: never
  showCloseButton?: boolean
  variant?: 'default' | 'palette'
}) {
  return (
    <Dialog {...props}>
      <DialogHeader sx={stylexStyles.rule1}>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <DialogContent
        sx={[
          variant === 'palette'
            ? commandDialogStyles.palette
            : commandDialogStyles.default,
          sx,
        ]}
        showCloseButton={showCloseButton}
      >
        <Command
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: **:data-[slot=command-input-wrapper]:h-10 */ '[&_[cmdk-group-heading]]:text-foreground-alt **:data-[slot=command-input-wrapper]:h-10 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group]]:px-1.5 [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0 [&_[cmdk-input-wrapper]_svg]:h-4 [&_[cmdk-input-wrapper]_svg]:w-4 [&_[cmdk-input]]:h-10 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-1.5 [&_[cmdk-item]_svg]:h-4 [&_[cmdk-item]_svg]:w-4'
          }
        >
          {children}
        </Command>
      </DialogContent>
    </Dialog>
  )
}

function CommandInput({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Input> &
  VariantProps<typeof commandInputVariants>) {
  return (
    <div
      data-slot="command-input-wrapper"
      className={
        /* TODO(stylex): computed class helper */ cn(
          'border-border/60 flex h-10 items-center gap-2 border-b px-3',
          commandInputWrapperVariants({ variant }),
        )
      }
    >
      <LuSearch sx={stylexStyles.rule2} />
      <CommandPrimitive.Input
        data-slot="command-input"
        className={
          /* TODO(stylex): computed class helper */ cn(
            'placeholder:text-foreground-alt/60 flex h-10 w-full bg-transparent outline-hidden disabled:cursor-not-allowed disabled:opacity-50',
            commandInputVariants({ variant }),
            className,
          )
        }
        {...props}
      />
    </div>
  )
}

function CommandList({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.List> &
  VariantProps<typeof commandListVariants>) {
  return (
    <CommandPrimitive.List
      data-slot="command-list"
      className={
        /* TODO(stylex): computed class helper */ cn(
          'min-h-0 flex-1 scroll-py-1 overflow-x-hidden overflow-y-auto',
          commandListVariants({ variant }),
          className,
        )
      }
      {...props}
    />
  )
}

function CommandEmpty({
  variant,
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Empty> &
  VariantProps<typeof commandEmptyVariants>) {
  return (
    <CommandPrimitive.Empty
      data-slot="command-empty"
      className={
        /* TODO(stylex): computed class helper */ cn(
          'text-foreground-alt/60 py-8 text-center text-sm',
          commandEmptyVariants({ variant }),
          className,
        )
      }
      {...props}
    />
  )
}

function CommandGroup({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Group> &
  VariantProps<typeof commandGroupVariants>) {
  return (
    <CommandPrimitive.Group
      data-slot="command-group"
      className={
        /* TODO(stylex): computed class helper */ cn(
          'text-foreground overflow-hidden p-1',
          commandGroupVariants({ variant }),
          className,
        )
      }
      {...props}
    />
  )
}

function CommandSeparator({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Separator> & {
  variant?: 'default' | 'subtle'
}) {
  return (
    <CommandPrimitive.Separator
      data-slot="command-separator"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'bg-border/60 -mx-1 h-px',
          variant === 'subtle' && 'bg-foreground/8',
          className,
        )
      }
      {...props}
    />
  )
}

function CommandItem({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Item> &
  VariantProps<typeof commandItemVariants>) {
  return (
    <IconStylesProvider sx={[iconDefaults.command]}>
      <CommandPrimitive.Item
        data-slot="command-item"
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: [&_svg]:pointer-events-none */ cn(
            'data-[selected=true]:bg-menu-selected data-[selected=true]:text-foreground relative flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-hidden transition-colors select-none data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0',
            commandItemVariants({ variant }),
            className,
            iconStylex.props(commandIconMarker).className,
          )
        }
        {...props}
      />
    </IconStylesProvider>
  )
}

function CommandShortcut({
  className,
  variant,
  ...props
}: React.ComponentProps<'span'> &
  VariantProps<typeof commandShortcutVariants>) {
  return (
    <span
      data-slot="command-shortcut"
      className={
        /* TODO(stylex): computed class helper */ cn(
          'text-foreground-alt/60 ml-auto font-mono text-metadata tracking-wide',
          commandShortcutVariants({ variant }),
          className,
        )
      }
      {...props}
    />
  )
}

function CommandFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="command-footer"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'border-border/60 text-foreground-alt/50 flex items-center justify-center gap-4 border-t px-3 py-1.5 micro-ten',
          className,
        )
      }
      {...props}
    >
      <span sx={stylexStyles.rule3}>
        <kbd sx={stylexStyles.rule4}>&#8593;&#8595;</kbd>
        Navigate
      </span>
      <span sx={stylexStyles.rule3}>
        <kbd sx={stylexStyles.rule4}>&#8629;</kbd>
        Select
      </span>
      <span sx={stylexStyles.rule3}>
        <kbd sx={stylexStyles.rule4}>Esc</kbd>
        Close
      </span>
    </div>
  )
}

export {
  Command,
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
  CommandSeparator,
  CommandFooter,
}
