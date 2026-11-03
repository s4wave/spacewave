import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import React, { useCallback } from 'react'
import * as Collapsible from '@radix-ui/react-collapsible'
import { LuChevronDown } from '@s4wave/web/images/icons.js'

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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  actions: { alignItems: 'center', display: 'flex', flexShrink: 0 },
  header: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    display: 'flex',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': '24px',
    },
  },
  compactHeader: {
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 1.5)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  regularHeader: {
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 2.5)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  openHeader: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  trigger: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    alignSelf: 'stretch',
    cursor: 'pointer',
    display: 'flex',
    textAlign: 'left',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': '24px',
    },
    minWidth: 0,
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
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  title: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    flex: 1,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    minWidth: 0,
  },
  chevron: {
    '--tw-duration': '.15s',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    flexShrink: 0,
    transitionDuration: '.15s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
                controlTargetStyles.minimum,
              ]}
            >
              {icon && <span sx={styles.icon}>{icon}</span>}
              <span sx={styles.title}>{title}</span>
              {badge}
              <LuChevronDown
                sx={[styles.chevron, open && styles.openChevron]}
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
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    justifyContent: 'center',
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': '24px',
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': '24px',
    },
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})
