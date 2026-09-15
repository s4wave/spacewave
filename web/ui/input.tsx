import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@s4wave/web/style/utils.js'

const inputVariants = cva('', {
  variants: {
    variant: {
      config:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9',
      configCompact:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9 text-xs',
      search:
        'border-foreground/10 bg-background/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 pl-9',
      searchPanel:
        'border-foreground/10 bg-background/30 focus-visible:border-brand/50 focus-visible:ring-brand/15 pl-9',
      command:
        'h-auto border-0 bg-transparent px-0 py-1 text-base shadow-none focus-visible:ring-0',
      displayName: 'text-sm',
      plugin:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-8 font-mono text-xs',
      toolbar:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-7 text-xs',
      configMono:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9 font-mono text-xs',
      wizard:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9 text-xs',
      wizardError:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 border-destructive/50 h-9 text-xs',
      wizardMono:
        'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9 font-mono text-xs',
    },
  },
})

interface InputProps
  extends React.ComponentProps<'input'>, VariantProps<typeof inputVariants> {}

function Input({ className, type, variant, ...props }: InputProps) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input h-9 w-full min-w-0 rounded-md border bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
        'focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-3',
        'aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive',
        inputVariants({ variant }),
        className,
      )}
      {...props}
    />
  )
}

export { Input }
