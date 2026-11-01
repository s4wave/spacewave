import { cn as combine } from 'cnfast'
import { cva } from 'class-variance-authority'

const stylexStyles = 'existing-binding'
const stylexStyles2 = 'another-binding'
const themeColors = 'existing-theme-binding'
const rowClasses = 'inline-flex items-center gap-2'
const recipe = cva('rounded-md px-2 py-1', {
  variants: {
    tone: { primary: 'text-brand', muted: 'text-foreground-alt' },
    compact: { true: 'text-xs', false: 'text-sm' },
  },
  defaultVariants: { tone: 'primary', compact: false },
  compoundVariants: [
    { tone: ['primary', 'muted'], compact: true, className: 'font-medium' },
  ],
})

// ExpressionCases keeps lexical constants, aliases and recipe defaults in one browser contract.
export function ExpressionCases() {
  return (
    <div data-existing={`${stylexStyles}-${stylexStyles2}-${themeColors}`}>
      <span
        className={combine(
          rowClasses,
          { 'select-none': true },
          recipe({ compact: true }),
        )}
      >
        Compact
      </span>
      <span className={combine(rowClasses, recipe({ tone: 'muted' }))}>
        Muted
      </span>
      <span className={combine(rowClasses, recipe({ tone: null }))}>
        Inherit
      </span>
    </div>
  )
}
