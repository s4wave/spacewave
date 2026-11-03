import { cn as combine } from 'cnfast'
import { cva } from 'class-variance-authority'

const stylexStyles = 'existing-binding'
const stylexStyles2 = 'another-binding'
const themeColors = 'existing-theme-binding'
const rowClasses = 'inline-flex items-center gap-2'
const recipe = cva(
  'rounded-md px-2 py-1 text-sm hover:text-foreground md:text-base',
  {
    variants: {
      tone: {
        primary: 'text-brand hover:text-brand',
        muted: 'text-foreground-alt',
      },
      compact: { true: 'text-xs', false: 'text-sm' },
    },
    defaultVariants: { tone: 'primary', compact: false },
    compoundVariants: [
      { tone: ['primary', 'muted'], compact: true, className: 'font-medium' },
    ],
  },
)

// ExpressionCases keeps lexical constants, aliases and recipe defaults in one browser contract.
export function ExpressionCases() {
  return (
    <div data-existing={`${stylexStyles}-${stylexStyles2}-${themeColors}`}>
      <div
        className="progress-width bg-brand h-2"
        style={{ '--progress-width': '35%' } as React.CSSProperties}
      />
      <div
        className="progress-width bg-brand h-2 w-20"
        style={{ '--progress-width': '35%' } as React.CSSProperties}
      />
      <div
        className="layout-debug-frame h-8"
        style={{ '--layout-debug-height': '80px' } as React.CSSProperties}
      >
        Utility height wins
      </div>
      <div className="layout-colors-surface bg-brand">Utility color wins</div>
      <span
        className={combine(
          true,
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
      <div className="group/code" data-numbered>
        <span className={"group-data-[numbered]/code:before:content-['#'] group-data-[numbered]/code:before:select-none"}>
          Related pseudo-element
        </span>
        <span className={"after:content-['−']"}>Unicode removal marker</span>
      </div>
      {[false, true].map((selected) => (
        <button
          type="button"
          key={String(selected)}
          className={combine(
            'inline-flex px-2',
            selected
              ? 'bg-brand/10 text-foreground'
              : 'text-foreground/90 hover:bg-foreground/5',
          )}
        >
          {selected ? 'Selected branch' : 'Unselected branch'}
        </button>
      ))}
    </div>
  )
}
