import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '2px',
  },
  rule3: { borderColor: themeColors['--color-brand'] },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
  },
  rule6: { color: themeColors['--color-foreground-alt'] },
  rule7: { color: themeColors['--color-brand'] },
  rule8: { minWidth: 0, flex: 1 },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule11: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule12: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule13: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

export interface RadioOptionProps {
  selected: boolean
  onSelect: () => void
  icon?: React.ReactNode
  label: React.ReactNode
  tag?: React.ReactNode
  description?: React.ReactNode
  disabled?: boolean
  className?: string
}

// RadioOption renders a selectable radio button card with optional icon and tag.
function RadioOption({
  selected,
  onSelect,
  icon,
  label,
  tag,
  description,
  disabled,
  className,
}: RadioOptionProps) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onSelect}
      disabled={disabled}
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'w-full rounded-md border p-2.5 text-left transition-all duration-200',
          'disabled:cursor-not-allowed disabled:opacity-60',
          selected
            ? 'border-brand/30 bg-brand/5'
            : 'border-foreground/10 bg-background/20 enabled:hover:border-foreground/20',
          className,
        )
      }
    >
      <div sx={stylexStyles.rule1}>
        <div
          sx={[
            stylexStyles.rule2,
            selected ? stylexStyles.rule3 : stylexStyles.rule4,
          ]}
        >
          {selected && <div sx={stylexStyles.rule5} />}
        </div>
        {icon && (
          <div sx={[stylexStyles.rule6, !!selected && stylexStyles.rule7]}>
            {icon}
          </div>
        )}
        <div sx={stylexStyles.rule8}>
          <div sx={stylexStyles.rule9}>
            <p sx={stylexStyles.rule10}>{label}</p>
            {tag && (
              <span
                sx={[
                  stylexStyles.rule11,
                  selected ? stylexStyles.rule12 : stylexStyles.rule13,
                ]}
              >
                {tag}
              </span>
            )}
          </div>
          {description && <p sx={stylexStyles.rule14}>{description}</p>}
        </div>
      </div>
    </button>
  )
}

export { RadioOption }
