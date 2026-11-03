import { useState } from 'react'
import { LuMenu, LuX } from '@s4wave/web/images/icons.js'

import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@s4wave/web/ui/sheet.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  radii as themeRadii,
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-topbar-back'],
    display: { default: 'flex', '@container (min-width: 42rem)': 'none' },
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    minWidth: 0,
  },
  rule2: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'inline-flex',
    flexShrink: 0,
    justifyContent: 'center',
    outlineStyle: { default: null, ':focus-visible': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    width: `calc(${themeLayout['--spacing']} * 11)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule4: { flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule6: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'inline-flex',
    justifyContent: 'center',
    outlineStyle: { default: null, ':focus-visible': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    width: `calc(${themeLayout['--spacing']} * 11)`,
  },
  rule7: { flex: 1, minHeight: 0, overflowY: 'auto' },
  rule8: {
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-landing'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    width: '100%',
  },
  rule9: { flex: 1, overflow: 'hidden', display: 'flex', minHeight: 0 },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
    flexShrink: 0,
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    overflowY: 'auto',
    width: `calc(${themeLayout['--spacing']} * 58)`,
  },
  rule11: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    overflowX: 'hidden',
    overflowY: 'auto',
  },
  rule12: {
    flex: 1,
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 5)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBottom: `calc(${themeLayout['--spacing']} * 20)`,
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    width: '100%',
  },
  rule13: { maxWidth: 'var(--container-5xl)' },
  rule14: { maxWidth: 'var(--container-3xl)' },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    maxWidth: {
      default: themeLayout['--max-width-docs-sidebar'],
      '@media (min-width: 40rem)': 'var(--container-sm)',
    },
    width: `calc(${themeLayout['--spacing']} * 70)`,
  },
})

// DocsLayoutProps defines the props for DocsLayout.
interface DocsLayoutProps {
  sidebar: React.ReactNode
  children: React.ReactNode
  currentSlug?: string
  mobileTitle?: string
  // contentWidth selects the content column measure. 'reading' is the prose
  // measure for article pages; 'wide' is the card-grid measure so hub and
  // section index pages fill a desktop viewport instead of stranding a gutter.
  contentWidth?: 'reading' | 'wide'
}

interface DocsMobileNavProps {
  portalContainer: HTMLDivElement | null
  sidebar: React.ReactNode
  title: string
}

// DocsMobileNav renders the full-width top bar with the hamburger trigger and
// the slide-in navigation drawer. It occupies its own row above the content so
// the trigger spans the viewport rather than sitting beside the content column.
function DocsMobileNav({
  portalContainer,
  sidebar,
  title,
}: DocsMobileNavProps) {
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
      <div sx={stylexStyles3.rule1}>
        <SheetTrigger asChild>
          <button
            type="button"
            aria-label="Open documentation navigation"
            sx={stylexStyles3.rule2}
          >
            <LuMenu sx={stylexStyles2.rule1} />
          </button>
        </SheetTrigger>
        <span sx={stylexStyles3.rule3}>{title}</span>
      </div>
      <SheetContent
        side="left"
        position="absolute"
        portalContainer={portalContainer}
        showCloseButton={false}
        variant="docs"
        sx={stylexStyles.rule1}
      >
        <div sx={stylexStyles3.rule4}>
          <div sx={stylexStyles3.rule5}>
            <SheetTitle variant="docsPlain">Documentation</SheetTitle>
            <SheetClose asChild>
              <button
                type="button"
                aria-label="Close documentation navigation"
                sx={stylexStyles3.rule6}
              >
                <LuX sx={stylexStyles2.rule1} />
              </button>
            </SheetClose>
          </div>
          <div sx={stylexStyles3.rule7}>{sidebar}</div>
        </div>
      </SheetContent>
    </Sheet>
  )
}

// DocsLayout renders the responsive documentation shell: a persistent left rail
// on wide displays that collapses into a top-bar drawer below the @2xl container
// breakpoint. The content column carries its own min-w-0 so wide code blocks and
// tables scroll within it instead of forcing the page to scroll sideways.
export function DocsLayout({
  sidebar,
  children,
  currentSlug,
  mobileTitle = 'Documentation',
  contentWidth = 'reading',
}: DocsLayoutProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLDivElement | null>(
    null,
  )

  return (
    <div ref={setPortalContainer} sx={stylexStyles3.rule8}>
      {/* Navigation drawer - narrow widths only */}
      <DocsMobileNav
        key={currentSlug ?? 'docs-root'}
        portalContainer={portalContainer}
        sidebar={sidebar}
        title={mobileTitle}
      />

      <div sx={stylexStyles3.rule9}>
        {/* Sidebar - wide widths only */}
        <aside sx={stylexStyles3.rule10}>{sidebar}</aside>

        {/* Content. overflow-x-hidden keeps the scroll region from ever
            scrolling sideways: wide prose scrolls inside its own bounded box
            and the shell never gains a horizontal scrollbar. */}
        <main sx={stylexStyles3.rule11}>
          <div
            sx={[
              stylexStyles3.rule12,
              contentWidth === 'wide'
                ? stylexStyles3.rule13
                : stylexStyles3.rule14,
            ]}
          >
            {children}
          </div>
          <LegalFooter />
        </main>
      </div>
    </div>
  )
}
