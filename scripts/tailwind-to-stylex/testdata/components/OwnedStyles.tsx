import React from 'react'
import { cn } from 'cnfast'

const ComponentStyles = 'existing-binding'

interface BoxProps extends React.ComponentProps<'div'> {
  label: string
}

// Box owns its default style while callers provide the final override.
function Box({ className, label, ...props }: BoxProps) {
  return (
    <div
      aria-label={label}
      className={cn('p-2 p-1 text-sm rounded-md', className)}
      {...props}
    />
  )
}

// OwnedStyles composes the same public styling input at two values.
export function OwnedStyles() {
  return (
    <section data-existing={ComponentStyles}>
      <Box label="Default box">Default</Box>
      <Box label="Overridden box" className="border border-dashed p-6 text-lg">
        Override
      </Box>
      <button
        type="button"
        data-state="open"
        className="data-[state=open]:bg-brand/10 px-2 py-1"
      >
        Attribute state
      </button>
    </section>
  )
}
