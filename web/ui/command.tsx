import * as React from 'react'
import { Command as CommandPrimitive } from 'cmdk'
import { LuSearch } from 'react-icons/lu'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog'

const commandVariants = cva('', {
  variants: {
    variant: {
      default: 'rounded-md',
      landing:
        'border-foreground/11 bg-background-get-started rounded-landing-launcher shadow-landing-launcher relative min-h-50 border backdrop-blur-sm',
      dashboard:
        'border-ui-outline bg-background-get-started/95 relative rounded-lg border shadow-xl backdrop-blur-sm',
      dashboardEmpty:
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

const commandGroupVariants = cva('', {
  variants: {
    variant: {
      default: '',
      compact: 'py-1',
      palette: '!px-0 [&_[cmdk-group-heading]]:px-3',
    },
  },
})

const commandItemVariants = cva('', {
  variants: {
    variant: {
      default: '',
      landing:
        'text-foreground-alt mx-1 flex rounded-lg cursor-pointer items-center gap-3 px-3 py-1.5 duration-200',
      dashboard:
        'group flex cursor-pointer items-center gap-3 rounded-md bg-transparent px-3 py-2.5',
      dashboardIdentifier:
        'group flex cursor-pointer items-center gap-3 rounded-md bg-transparent px-3 py-2.5 pr-16',
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
      className={cn(
        'bg-popover text-popover-foreground flex max-h-[inherit] w-full flex-col overflow-hidden',
        commandVariants({ variant }),
        className,
      )}
      {...props}
    />
  )
}

function CommandDialog({
  title = 'Command Palette',
  description = 'Search for a command to run...',
  children,
  className,
  showCloseButton = true,
  variant,
  ...props
}: React.ComponentProps<typeof Dialog> & {
  title?: string
  description?: string
  className?: string
  showCloseButton?: boolean
  variant?: 'default' | 'palette'
}) {
  return (
    <Dialog {...props}>
      <DialogHeader className="sr-only">
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <DialogContent
        className={cn(
          'top-[38%] max-h-[min(28rem,calc(100vh-6rem))] overflow-hidden p-0 sm:max-w-xl',
          variant === 'palette' &&
            'border-foreground/10 bg-background-card/95 bottom-4 top-auto max-h-(--max-height-command-palette) w-(--width-command-editor) translate-y-0 shadow-none sm:max-w-none',
          className,
        )}
        showCloseButton={showCloseButton}
      >
        <Command className="[&_[cmdk-group-heading]]:text-foreground-alt **:data-[slot=command-input-wrapper]:h-10 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group]]:px-1.5 [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0 [&_[cmdk-input-wrapper]_svg]:h-4 [&_[cmdk-input-wrapper]_svg]:w-4 [&_[cmdk-input]]:h-10 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-1.5 [&_[cmdk-item]_svg]:h-4 [&_[cmdk-item]_svg]:w-4">
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
      className={cn(
        'border-border/60 flex h-10 items-center gap-2 border-b px-3',
        commandInputWrapperVariants({ variant }),
      )}
    >
      <LuSearch className="text-foreground-alt size-4 shrink-0" />
      <CommandPrimitive.Input
        data-slot="command-input"
        className={cn(
          'placeholder:text-foreground-alt/60 flex h-10 w-full bg-transparent outline-hidden disabled:cursor-not-allowed disabled:opacity-50',
          commandInputVariants({ variant }),
          className,
        )}
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
      className={cn(
        'min-h-0 flex-1 scroll-py-1 overflow-x-hidden overflow-y-auto',
        commandListVariants({ variant }),
        className,
      )}
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
      className={cn(
        'text-foreground-alt/60 py-8 text-center text-sm',
        commandEmptyVariants({ variant }),
        className,
      )}
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
      className={cn(
        'text-foreground [&_[cmdk-group-heading]]:text-foreground-alt/70 overflow-hidden p-1 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:micro-ten [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-widest [&_[cmdk-group-heading]]:uppercase',
        commandGroupVariants({ variant }),
        className,
      )}
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
      className={cn(
        'bg-border/60 -mx-1 h-px',
        variant === 'subtle' && 'bg-foreground/8',
        className,
      )}
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
    <CommandPrimitive.Item
      data-slot="command-item"
      className={cn(
        "data-[selected=true]:bg-menu-selected data-[selected=true]:text-foreground [&_svg:not([class*='text-'])]:text-foreground-alt relative flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-hidden transition-colors select-none data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        commandItemVariants({ variant }),
        className,
      )}
      {...props}
    />
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
      className={cn(
        'text-foreground-alt/60 ml-auto font-mono text-metadata tracking-wide',
        commandShortcutVariants({ variant }),
        className,
      )}
      {...props}
    />
  )
}

function CommandFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="command-footer"
      className={cn(
        'border-border/60 text-foreground-alt/50 flex items-center justify-center gap-4 border-t px-3 py-1.5 micro-ten',
        className,
      )}
      {...props}
    >
      <span className="flex items-center gap-1">
        <kbd className="bg-muted/50 micro-ten rounded px-1 py-0.5 font-mono leading-none">
          &#8593;&#8595;
        </kbd>
        Navigate
      </span>
      <span className="flex items-center gap-1">
        <kbd className="bg-muted/50 micro-ten rounded px-1 py-0.5 font-mono leading-none">
          &#8629;
        </kbd>
        Select
      </span>
      <span className="flex items-center gap-1">
        <kbd className="bg-muted/50 micro-ten rounded px-1 py-0.5 font-mono leading-none">
          Esc
        </kbd>
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
