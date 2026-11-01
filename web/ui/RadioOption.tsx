import type { StyleXStyles } from '@stylexjs/stylex'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  root: {
    width: '100%',
    borderRadius: 'var(--radius-md)',
    borderWidth: '1px',
    borderStyle: themeUtilityState['--tw-border-style'],
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.6 },
  },
  selected: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  unselected: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':enabled:hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
  },
  row: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  radio: {
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
  selectedBorder: { borderColor: themeColors['--color-brand'] },
  unselectedBorder: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
  },
  dot: {
    width: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
  },
  icon: { color: themeColors['--color-foreground-alt'] },
  selectedIcon: { color: themeColors['--color-brand'] },
  body: { minWidth: 0, flex: 1 },
  heading: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  label: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  tag: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  selectedTag: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    color: themeColors['--color-brand'],
  },
  unselectedTag: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  description: {
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
  sx?: StyleXStyles
  className?: never
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
  sx,
}: RadioOptionProps) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onSelect}
      disabled={disabled}
      sx={[styles.root, selected ? styles.selected : styles.unselected, sx]}
    >
      <div sx={styles.row}>
        <div
          sx={[
            styles.radio,
            selected ? styles.selectedBorder : styles.unselectedBorder,
          ]}
        >
          {selected && <div sx={styles.dot} />}
        </div>
        {icon && (
          <div sx={[styles.icon, !!selected && styles.selectedIcon]}>
            {icon}
          </div>
        )}
        <div sx={styles.body}>
          <div sx={styles.heading}>
            <p sx={styles.label}>{label}</p>
            {tag && (
              <span
                sx={[
                  styles.tag,
                  selected ? styles.selectedTag : styles.unselectedTag,
                ]}
              >
                {tag}
              </span>
            )}
          </div>
          {description && <p sx={styles.description}>{description}</p>}
        </div>
      </div>
    </button>
  )
}

export { RadioOption }
