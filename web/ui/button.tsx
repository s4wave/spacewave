import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@s4wave/web/style/utils.js'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground shadow hover:bg-primary/90',
        destructive:
          'bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90',
        outline:
          'border border-input bg-background shadow-sm hover:bg-accent hover:text-accent-foreground',
        secondary:
          'bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/80',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
        brandOutline:
          'border border-brand/30 bg-brand/10 text-foreground hover:border-brand/50 hover:bg-brand/15',
        quiet:
          'border-foreground/8 bg-transparent text-foreground-alt hover:border-foreground/15 hover:bg-foreground/5 hover:text-foreground',
        destructiveOutline:
          'border border-destructive/20 bg-destructive/10 text-destructive hover:bg-destructive/15',
        dangerGhost:
          'text-destructive/70 hover:bg-destructive/10 hover:text-destructive',
        brandGhost: 'bg-brand/10 text-brand hover:bg-brand/15',
        muted: 'text-foreground-alt/60 hover:text-foreground',
        brandSoft:
          'border border-brand/60 bg-brand/25 text-foreground hover:border-brand/80 hover:bg-brand/35',
        selectedBrand: 'border border-brand/20 bg-brand/10 hover:bg-brand/10',
        capture:
          'border-foreground/10 bg-background/40 font-normal hover:border-foreground/20 data-[capturing=true]:border-brand/50 data-[capturing=true]:bg-brand/10 data-[capturing=true]:text-brand data-[capturing=true]:ring-brand/20 data-[capturing=true]:ring-4',
        unselectedRow:
          'border border-transparent hover:border-foreground/8 hover:bg-foreground/3',
        selectedRow: 'border border-brand/30 bg-brand/10 hover:bg-brand/10',
        debugOutline:
          'border-foreground/10 bg-background/30 hover:border-brand/40 hover:bg-brand/10',
        debugGhost: 'hover:bg-foreground/3',
        destructiveSubtle:
          'border border-foreground/8 bg-transparent hover:border-destructive/30 hover:bg-destructive/5 hover:text-destructive',
        errorGhost: 'text-error hover:text-error',
        selectedFinder:
          'rounded-lg border border-brand/20 bg-brand/10 hover:bg-brand/10',
        unselectedFinder:
          'rounded-lg border border-transparent hover:bg-foreground/4',
        category:
          'border-foreground/8 bg-background-card/30 text-foreground hover:bg-foreground/5',
      },
      size: {
        default: 'h-9 px-4 py-2',
        sm: 'h-8 rounded-md px-3 text-xs',
        lg: 'h-10 rounded-md px-8',
        icon: 'h-9 w-9',
        xs: 'h-6 gap-1 px-2 text-xs',
        toolbar: 'h-7 gap-1 text-xs',
        toolbarSm: 'h-7 px-2 text-xs',
        toolbarWide: 'h-7 px-3 text-xs',
        brandWide: 'h-7 gap-1.5 px-3 text-xs',
        toolbarText: 'h-7 text-xs',
        compact8: 'h-8 px-2 text-xs',
        iconSm: 'h-8 w-8',
        iconXs: 'h-6 w-6 p-0',
        row: 'h-auto gap-3 rounded-lg border p-3',
        finderRow: 'h-auto rounded-lg px-3 py-2.5',
        commandRow: 'h-auto px-2.5 py-2',
        category: 'h-auto rounded-none px-3.5 py-3',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export interface ButtonProps
  extends
    React.ComponentPropsWithRef<'button'>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot : 'button'
  return (
    <Comp
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}
Button.displayName = 'Button'

export { Button, buttonVariants }
