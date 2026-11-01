import React, { useCallback } from 'react'
import * as Collapsible from '@radix-ui/react-collapsible'
import { LuChevronDown } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
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
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule4: { rotate: '180deg' },
  rule5: { padding: `calc(${themeLayout['--spacing']} * 2.5)` },
  rule6: { padding: `calc(${themeLayout['--spacing']} * 3.5)` },
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
  // Optional class name for the outer container.
  className?: string
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
  className,
  badge,
  headerActions,
  compact = false,
}: CollapsibleSectionProps) {
  const toggle = useCallback(() => onOpenChange(!open), [open, onOpenChange])

  return (
    <Collapsible.Root open={open} onOpenChange={onOpenChange} asChild>
      <section
        className={
          /* TODO(stylex): dynamic class or forwarded className */ cn(
            'border-foreground/6 bg-background-card/30 rounded-lg border',
            className,
          )
        }
      >
        <div
          className={
            /* TODO(stylex): conditional property composition: paddingBlock */ cn(
              'hover:bg-background-card/50 flex min-h-11 items-center gap-2 transition-colors fine-pointer:min-h-0',
              compact ? 'px-2.5 sm:py-1.5' : 'px-3.5 sm:py-2.5',
              open && 'border-foreground/6 border-b',
            )
          }
        >
          <Collapsible.Trigger asChild>
            <button
              type="button"
              onClick={toggle}
              className={
                /* TODO(stylex): conditional property composition: marginBlock */ cn(
                  'flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch text-left fine-pointer:min-h-0',
                  compact ? 'sm:-my-1.5 sm:py-1.5' : 'sm:-my-2.5 sm:py-2.5',
                )
              }
            >
              {icon && <span sx={stylexStyles.rule1}>{icon}</span>}
              <span sx={stylexStyles.rule2}>{title}</span>
              {badge}
              <LuChevronDown
                {...stylex.props([
                  stylexStyles.rule3,
                  !!open && stylexStyles.rule4,
                ])}
              />
            </button>
          </Collapsible.Trigger>
          {headerActions && (
            <div
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: [&_button]:min-h-11 */ 'fine-pointer:[&_button]:min-h-0 fine-pointer:[&_button]:min-w-0 flex shrink-0 items-center [&_button]:min-h-11 [&_button]:min-w-11'
              }
            >
              {headerActions}
            </div>
          )}
        </div>
        <Collapsible.Content
          className={
            /* TODO(stylex): global or unknown class: data-[state=closed]:animate-collapsible-up */ 'data-[state=closed]:animate-collapsible-up data-[state=open]:animate-collapsible-down overflow-hidden'
          }
        >
          <div sx={[compact ? stylexStyles.rule5 : stylexStyles.rule6]}>
            {children}
          </div>
        </Collapsible.Content>
      </section>
    </Collapsible.Root>
  )
}
