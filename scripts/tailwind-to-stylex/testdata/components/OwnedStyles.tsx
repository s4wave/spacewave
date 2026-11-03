import React from 'react'
import { cn } from 'cnfast'
import * as compiled from '@stylexjs/stylex'
import { Label } from './label.js'

const ComponentStyles = 'existing-binding'
const compiledStyles = compiled.create({
  override: { padding: 12 },
  dynamic: (minWidth: number) => ({ minWidth }),
})

interface BoxProps extends React.ComponentProps<'div'> {
  label?: string
}

// Box owns its default style while callers provide the final override.
function Box({
  className = '',
  label = 'Default box',
  ...props
}: BoxProps = {}) {
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
      {[false, true].map((disabled) => (
        <div
          key={String(disabled)}
          className="group flex gap-2"
          data-disabled={disabled}
        >
          <Label>Ancestor {String(disabled)}</Label>
          <input
            aria-label={'Peer ' + disabled}
            type="checkbox"
            className="peer"
            disabled={disabled}
          />
          <Label>Following peer</Label>
          <span>Independent sibling</span>
        </div>
      ))}
      <div className="group/code flex gap-2">
        <button type="button" aria-label="Code hover">
          Hover code
        </button>
        <span className="group-hover/code:text-brand">Code action</span>
        <div className="group/code-block">
          <span className="group-hover/code-block:opacity-50">
            Nested code action
          </span>
        </div>
      </div>
      {[false, true].map((expanded) => (
        <div
          key={String(expanded)}
          className="group/trace-collapsible-trigger"
          aria-expanded={expanded}
        >
          <span className="inline-flex group-aria-expanded/trace-collapsible-trigger:rotate-90">
            Expansion indicator
          </span>
        </div>
      ))}
      <div
        aria-label="Mixed native styles"
        className="p-1"
        sx={compiledStyles.override}
      >
        Mixed native styles
      </div>
      {[false, true].map((selected) => (
        <button
          key={String(selected)}
          aria-label={'Finite selected ' + selected}
          className={cn(
            'border border-foreground/10 hover:border-foreground/20 hover:bg-foreground/5 p-2',
            selected && 'border-brand/30 bg-brand/10',
          )}
        >
          Finite {String(selected)}
        </button>
      ))}
      <div
        aria-label="Unlayered font alias"
        className="text-text-secondary text-lg leading-tight tracking-wider sm:text-2xl"
      >
        Alias typography
      </div>
      {[false, true].map((active) => (
        <div
          key={'alias' + active}
          className={cn('text-lg text-text-secondary', active && 'text-brand')}
        >
          Alias color {String(active)}
        </div>
      ))}
      <div className="text-text-secondary before:text-xl before:content-['Alias']">
        Alias pseudo-element
      </div>
      <Box />
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
