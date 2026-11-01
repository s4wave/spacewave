import React, { useCallback } from 'react'
import * as Collapsible from '@radix-ui/react-collapsible'
import { LuChevronDown } from 'react-icons/lu'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
  utilityState as themeUtilityState,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  section: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  actions: { display: 'flex', flexShrink: 0, alignItems: 'center' },
  header: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
  },
  compactHeader: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 1.5)`,
    },
  },
  regularHeader: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 2.5)`,
    },
  },
  openHeader: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
  },
  trigger: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
    minWidth: 0,
    flex: 1,
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignSelf: 'stretch',
    textAlign: 'left',
  },
  compactTrigger: {
    marginBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * -1.5)`,
    },
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 1.5)`,
    },
  },
  regularTrigger: {
    marginBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * -2.5)`,
    },
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 2.5)`,
    },
  },
  icon: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  title: {
    minWidth: 0,
    flex: 1,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  chevron: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  openChevron: { rotate: '180deg' },
  compactContent: { padding: `calc(${themeLayout['--spacing']} * 2.5)` },
  regularContent: { padding: `calc(${themeLayout['--spacing']} * 3.5)` },
  content: { overflow: 'hidden' },
})

/** CollapsibleSectionProps configures a section and its optional header actions. */
export interface CollapsibleSectionProps {
  // Section title displayed in the header.
  title: string
  // Icon rendered before the title.
  icon?: React.ReactNode
  // Whether the section is currently open.
  open: boolean
  // Called when the open state changes.
  onOpenChange: (open: boolean) => void
  // Content rendered when expanded.
  children: React.ReactNode
  // sx overrides the outer container styles.
  sx?: stylex.StyleXStyles
  className?: never
  // Optional badge or count rendered after the title.
  badge?: React.ReactNode
  // Optional actions rendered in the header outside the trigger button.
  headerActions?: React.ReactNode
  // Tightens header and content padding for a narrow container. Off by default.
  compact?: boolean
}

/** CollapsibleSection renders a section whose header toggles its content. */
export function CollapsibleSection({
  title,
  icon,
  open,
  onOpenChange,
  children,
  sx,
  badge,
  headerActions,
  compact = false,
}: CollapsibleSectionProps) {
  const toggle = useCallback(() => onOpenChange(!open), [open, onOpenChange])

  return (
    <Collapsible.Root open={open} onOpenChange={onOpenChange} asChild>
      <section sx={[styles.section, sx]}>
        <div
          sx={[
            styles.header,
            compact ? styles.compactHeader : styles.regularHeader,
            open && styles.openHeader,
          ]}
        >
          <Collapsible.Trigger asChild>
            <button
              type="button"
              onClick={toggle}
              sx={[
                styles.trigger,
                compact ? styles.compactTrigger : styles.regularTrigger,
              ]}
            >
              {icon && <span sx={styles.icon}>{icon}</span>}
              <span sx={styles.title}>{title}</span>
              {badge}
              <LuChevronDown
                {...stylex.props([styles.chevron, open && styles.openChevron])}
              />
            </button>
          </Collapsible.Trigger>
          {headerActions && <div sx={styles.actions}>{headerActions}</div>}
        </div>
        <Collapsible.Content {...stylex.props(styles.content)}>
          <div sx={[compact ? styles.compactContent : styles.regularContent]}>
            {children}
          </div>
        </Collapsible.Content>
      </section>
    </Collapsible.Root>
  )
}

/** collapsibleSectionActionStyles styles a header action with its pointer-specific touch target. */
export const collapsibleSectionActionStyles = stylex.create({
  button: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})
