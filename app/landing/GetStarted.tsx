import React from 'react'
import { useMemo, useRef, useCallback, useEffect } from 'react'
import { LuFolderOpen, LuUser } from '@s4wave/web/images/icons.js'

import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { useAddSpaceRootAlias } from '@s4wave/app/hooks/useAddSpaceRootAlias.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import {
  getVisibleQuickstartOptions,
  getQuickstartPath,
  isQuickstartOptionPublic,
  type QuickstartOption,
} from '../quickstart/options.js'
import { useExperimentalCreatorsEnabled } from '../creator-visibility.js'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@s4wave/web/ui/command.js'
import { useIsStaticMode } from '../prerender/StaticContext.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  colors as themeColors,
  motion as themeMotion,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 0 0 1px var(--tw-shadow-color, oklch(0% 0 0 / .35)), 0 48px 96px -40px var(--tw-shadow-color, oklch(0% 0 0 / .8)), 0 14px 32px -18px var(--tw-shadow-color, oklch(0% 0 0 / .55))`,
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 11%, transparent)`,
    borderRadius: themeRadii['--radius-landing-launcher'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: { default: null, '@container (min-width: 32rem)': '0 auto' },
    overflow: { default: null, '@container (min-width: 32rem)': 'hidden' },
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    maxHeight: {
      default: null,
      '@container (min-width: 32rem)': themeLayout['--max-height-get-started'],
      '@container (min-width: 42rem)':
        themeLayout['--max-height-get-started-wide'],
    },
    minHeight: `calc(${themeLayout['--spacing']} * 50)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)), inset 0 0 0 1px var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent))`,
    borderRadius: themeRadii['--radius-landing-control'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: 0.6,
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3.25)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4.5)`,
    alignItems: 'center',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 70%, transparent)`,
    fontSize: themeLayout['--text-landing-prompt'],
  },
  rule6: { whiteSpace: 'nowrap' },
  rule7: {
    display: { default: 'none', '@media (min-width: 40rem)': 'inline' },
  },
  rule8: {
    flex: 1,
    alignContent: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': 'flex-start',
      },
    },
    backgroundColor: themeColors['--color-background-get-started'],
    display: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': 'grid',
      },
    },
    gridTemplateColumns: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
      },
    },
    minHeight: { default: null, '@container (min-width: 32rem)': 0 },
    overflowY: { default: null, '@container (min-width: 32rem)': 'auto' },
    paddingBottom: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule9: {
    order: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': -9999,
      },
    },
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    paddingBottom: themeLayout['--spacing'],
    paddingTop: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule11: {
    '--tw-duration': '.2s',
    borderRadius: themeRadii['--radius-lg'],
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    marginInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    textDecorationLine: 'none',
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule12: {
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)), inset 0 0 0 1px var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent))`,
    borderRadius: themeRadii['--radius-landing-control'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    alignContent: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': 'flex-start',
      },
    },
    display: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': 'grid',
      },
    },
    gridTemplateColumns: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
      },
    },
  },
  rule2: {
    order: {
      default: null,
      '@media (max-height: 470px)': {
        default: null,
        '@container (min-width: 32rem)': -9999,
      },
    },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    strokeWidth: '1.5px',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    strokeWidth: '1.5px',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

declare global {
  var __swStaticHandoffLinks: boolean | undefined
}

const BUILD_COMMAND_ITEMS = getVisibleQuickstartOptions(false)

function shouldUseStaticHandoffLinks(): boolean {
  return globalThis.__swStaticHandoffLinks === true
}

function getStaticQuickstartHref(
  item: QuickstartOption,
  useHandoffLinks: boolean,
): string {
  const path = getQuickstartPath(item)
  if (useHandoffLinks) return `#${path}`
  return isQuickstartOptionPublic(item, false) ? path : `#${path}`
}

const GetStartedItem = ({ item }: { item: QuickstartOption }) => {
  const navigate = useNavigate()
  const handleClick = useCallback(() => {
    const path = getQuickstartPath(item)
    navigate({ path })
  }, [item, navigate])

  return (
    <CommandItem key={item.id} variant="landing" onSelect={handleClick}>
      <div sx={stylexStyles4.rule1}>
        <item.icon sx={stylexStyles2.rule1} />
      </div>
      <div>
        <div sx={stylexStyles4.rule2}>{item.name}</div>
        <div sx={stylexStyles4.rule3}>{item.description}</div>
      </div>
    </CommandItem>
  )
}

function AddStateRootItem() {
  const { add: addRootAlias, canAdd: canAddRootAlias } = useAddSpaceRootAlias()
  const navigate = useNavigate()
  const handleClick = useCallback(async () => {
    if (await addRootAlias()) navigate({ path: '/sessions' })
  }, [addRootAlias, navigate])

  return (
    <CommandItem
      variant="landing"
      disabled={!canAddRootAlias}
      onSelect={handleClick}
    >
      <div sx={stylexStyles4.rule1}>
        <LuFolderOpen sx={stylexStyles.rule1} />
      </div>
      <div>
        <div sx={stylexStyles4.rule2}>Open a local state root</div>
        <div sx={stylexStyles4.rule3}>Add an existing .spacewave directory</div>
      </div>
    </CommandItem>
  )
}

function SessionItem({ session }: { session: SessionListEntry }) {
  // Resolve the account label and destination from the watched session metadata.
  const navigate = useNavigate()
  const metadata = useSessionMetadata(session.sessionIndex ?? null)
  const accountId =
    session.sessionRef?.providerResourceRef?.providerAccountId ?? 'Unknown'
  const accountName =
    metadata?.displayName || metadata?.cloudEntityId || accountId
  const providerLabel =
    metadata?.providerDisplayName ||
    (metadata?.providerId === 'spacewave'
      ? 'Cloud'
      : metadata?.providerId === 'local'
        ? 'Local'
        : 'Account')
  const subtitle =
    metadata?.cloudEntityId && metadata.cloudEntityId !== accountName
      ? `${providerLabel} · ${metadata.cloudEntityId}`
      : providerLabel

  // Read the handoff mode and group the static quickstart links.
  const handleClick = useCallback(() => {
    navigate({ path: '/u/' + session.sessionIndex + '/' })
  }, [navigate, session.sessionIndex])

  return (
    <CommandItem
      key={session.sessionIndex}
      variant="landing"
      onSelect={handleClick}
    >
      <div sx={stylexStyles4.rule1}>
        <LuUser sx={stylexStyles.rule1} />
      </div>
      <div>
        <div sx={stylexStyles4.rule2}>Account: {accountName}</div>
        <div sx={stylexStyles4.rule3}>{subtitle}</div>
      </div>
    </CommandItem>
  )
}

interface GetStartedProps {
  sx?: import('@stylexjs/stylex').StyleXStyles
  sessions?: SessionListEntry[]
}

// StaticGetStarted renders plain HTML links for the prerendered page.
// cmdk requires React effects to register items, so it shows "No templates
// found" during SSR. This version is crawlable and clickable without JS.
function StaticGetStarted({ sx }: Pick<GetStartedProps, 'sx'>) {
  const useHandoffLinks = shouldUseStaticHandoffLinks()
  const itemsByCategory = useMemo(() => {
    const categories = [
      ...new Set(BUILD_COMMAND_ITEMS.map((item) => item.category)),
    ]
    return categories.map((category) => ({
      category,
      items: BUILD_COMMAND_ITEMS.filter((item) => item.category === category),
    }))
  }, [])

  return (
    <div sx={[[stylexStyles5.rule1], sx]}>
      <div sx={stylexStyles4.rule4}>
        <span sx={stylexStyles4.rule5}>
          <span sx={stylexStyles4.rule6}>Where would you like to start?</span>
          <span sx={stylexStyles4.rule7}>
            {' '}
            Type here to get started instantly.
          </span>
        </span>
      </div>
      <div sx={stylexStyles4.rule8}>
        {itemsByCategory.map(({ category, items }) => (
          <div
            key={category}
            sx={[category === 'storage' && stylexStyles4.rule9]}
          >
            <div sx={stylexStyles4.rule10}>
              {category.charAt(0).toUpperCase() + category.slice(1)}
            </div>
            {items.map((item) => (
              <a
                key={item.id}
                href={getStaticQuickstartHref(item, useHandoffLinks)}
                sx={stylexStyles4.rule11}
              >
                <div sx={stylexStyles4.rule12}>
                  <item.icon sx={stylexStyles2.rule1} />
                </div>
                <div>
                  <div sx={stylexStyles4.rule2}>{item.name}</div>
                  <div sx={stylexStyles4.rule3}>{item.description}</div>
                </div>
              </a>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

const GetStarted = ({ sx, sessions }: GetStartedProps) => {
  // Resolve the launcher mode, available templates and input lifetime.
  const isStatic = useIsStaticMode()
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled()
  const inputRef = useRef<HTMLInputElement>(null)
  const itemsByCategory = useMemo(() => {
    const items = getVisibleQuickstartOptions(experimentalCreatorsEnabled)
    const categories = [...new Set(items.map((item) => item.category))]
    return categories.map((category) => ({
      category,
      items: items.filter((item) => item.category === category),
    }))
  }, [experimentalCreatorsEnabled])

  // Set up global event listener for Shift+Tab from nav links back to command input
  useEffect(() => {
    const handleGlobalKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Tab' && e.shiftKey) {
        const activeElement = document.activeElement
        if (
          activeElement &&
          activeElement.tagName === 'A' &&
          activeElement.closest('nav')
        ) {
          e.preventDefault()
          if (inputRef.current) {
            inputRef.current.focus()
          }
        }
      }
    }

    document.addEventListener('keydown', handleGlobalKeyDown, true)
    return () =>
      document.removeEventListener('keydown', handleGlobalKeyDown, true)
  }, [])

  // Move forward Tab navigation from the launcher to the page links.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Tab' && !e.shiftKey) {
        e.preventDefault()
        // Find the first navigation link and focus it
        const firstNavLink = document.querySelector('nav a')
        if (firstNavLink instanceof HTMLElement) {
          firstNavLink.focus()
        }
      }
    },
    [],
  )

  // Render crawlable links during prerendering and searchable commands after hydration.
  if (isStatic) return <StaticGetStarted sx={sx} />

  return (
    <Command variant="landing" sx={sx}>
      <CommandInput
        ref={inputRef}
        variant="landing"
        placeholder="Where would you like to start? Type here to get started instantly."
        onKeyDown={handleKeyDown}
      />
      <CommandList variant="landing" sx={stylexStyles3.rule1}>
        <CommandEmpty>No templates found.</CommandEmpty>
        {sessions && sessions.length > 0 && (
          <CommandGroup heading="Sessions" variant="landing">
            {sessions.map((session) => (
              <SessionItem key={session.sessionIndex} session={session} />
            ))}
          </CommandGroup>
        )}
        {itemsByCategory.map(({ category, items }) => (
          <CommandGroup
            key={category}
            heading={category.charAt(0).toUpperCase() + category.slice(1)}
            variant="landing"
            sx={[category === 'storage' && stylexStyles3.rule2]}
          >
            {items.map((item, idx) => (
              <React.Fragment key={item.id}>
                <GetStartedItem item={item} />
                {category === 'account' && idx === 1 && <AddStateRootItem />}
              </React.Fragment>
            ))}
          </CommandGroup>
        ))}
      </CommandList>
    </Command>
  )
}

export default GetStarted
