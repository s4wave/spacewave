import React from 'react'
import { cn } from 'cnfast'
import * as compiled from '@stylexjs/stylex'

const ComponentStyles = 'existing-binding'
const compiledStyles = compiled.create({
  override: { padding: 12 },
  dynamic: (minWidth: number) => ({ minWidth }),
})

interface BoxProps extends React.ComponentProps<'div'> {
  label: string
}

// Box owns its default style while callers provide the final override.
function Box({ className = '', label, ...props }: BoxProps) {
  return (
    <div
      aria-label={label}
      className={cn('p-2 p-1 text-sm rounded-md', className)}
      {...props}
    />
  )
}

// PassThrough forwards ordinary native props without owning a styling API.
function PassThrough(props: React.ComponentProps<'div'>) {
  return <div {...props} />
}

const passedProps: React.ComponentProps<typeof PassThrough> = {
  className: 'p-2',
}

// OwnedStyles composes the same public styling input at two values.
export function OwnedStyles({ optional = false }: { optional?: boolean } = {}) {
  return (
    <section data-existing={ComponentStyles}>
      <Box label="Default box">Default</Box>
      <Box
        label="Dynamic native caller"
        className="min-w-[123px]"
        style={{ '--owned-native-test': 'retained' }}
      >
        Dynamic caller
      </Box>
      <Box label="Compiled caller" {...compiled.props(compiledStyles.override)}>
        Compiled
      </Box>
      <Box label="Optional styles" className={optional ? 'p-6' : undefined}>
        Optional
      </Box>
      <PassThrough {...passedProps}>Native pass-through</PassThrough>
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
