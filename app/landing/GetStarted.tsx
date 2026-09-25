import React from 'react'
import { useMemo, useRef, useCallback, useEffect } from 'react'
import { LuFolderOpen, LuUser } from 'react-icons/lu'

import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { useAddSpaceRootAlias } from '@s4wave/app/hooks/useAddSpaceRootAlias.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { cn } from '@s4wave/web/style/utils.js'
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
      <div className="bg-foreground/10 text-foreground rounded-landing-control shadow-landing-tile flex size-9 shrink-0 items-center justify-center transition-colors">
        <item.icon className="icon-stroke-thin size-5" />
      </div>
      <div>
        <div className="text-sm font-medium">{item.name}</div>
        <div className="text-xs opacity-60">{item.description}</div>
      </div>
    </CommandItem>
  )
}

function AddStateRootItem() {
  const { add: addRootAlias, canAdd: canAddRootAlias } = useAddSpaceRootAlias()
  const handleClick = useCallback(() => {
    void addRootAlias()
  }, [addRootAlias])

  return (
    <CommandItem
      variant="landing"
      disabled={!canAddRootAlias}
      onSelect={handleClick}
    >
      <div className="bg-foreground/10 text-foreground rounded-landing-control shadow-landing-tile flex size-9 shrink-0 items-center justify-center transition-colors">
        <LuFolderOpen className="icon-stroke-thin size-5" />
      </div>
      <div>
        <div className="text-sm font-medium">Open a local state root</div>
        <div className="text-xs opacity-60">
          Add an existing .spacewave directory
        </div>
      </div>
    </CommandItem>
  )
}

function SessionItem({ session }: { session: SessionListEntry }) {
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

  const handleClick = useCallback(() => {
    navigate({ path: '/u/' + session.sessionIndex + '/' })
  }, [navigate, session.sessionIndex])

  return (
    <CommandItem
      key={session.sessionIndex}
      variant="landing"
      onSelect={handleClick}
    >
      <div className="bg-foreground/10 text-foreground rounded-landing-control shadow-landing-tile flex size-9 shrink-0 items-center justify-center transition-colors">
        <LuUser className="icon-stroke-thin size-5" />
      </div>
      <div>
        <div className="text-sm font-medium">Account: {accountName}</div>
        <div className="text-xs opacity-60">{subtitle}</div>
      </div>
    </CommandItem>
  )
}

interface GetStartedProps {
  className?: string
  sessions?: SessionListEntry[]
}

// StaticGetStarted renders plain HTML links for the prerendered page.
// cmdk requires React effects to register items, so it shows "No templates
// found" during SSR. This version is crawlable and clickable without JS.
function StaticGetStarted({ className }: { className?: string }) {
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
    <div
      className={cn(
        'border-foreground/11 bg-background-get-started rounded-landing-launcher shadow-landing-launcher relative flex min-h-50 flex-col border backdrop-blur-sm @lg:flex-1 @lg:overflow-hidden',
        '@lg:max-h-(--max-height-get-started) @lg:flex-initial @2xl:max-h-(--max-height-get-started-wide)',
        className,
      )}
    >
      <div className="border-foreground/7 flex items-center gap-2 border-b px-4.5 py-3.25">
        <span className="text-foreground/70 text-landing-prompt">
          <span className="whitespace-nowrap">
            Where would you like to start?
          </span>
          <span className="hidden sm:inline">
            {' '}
            Type here to get started instantly.
          </span>
        </span>
      </div>
      <div className="bg-background-get-started flex-1 pb-2.5 @lg:min-h-0 @lg:overflow-y-auto">
        {itemsByCategory.map(({ category, items }) => (
          <div key={category}>
            <div className="text-foreground/50 px-5 pt-2.5 pb-1 text-xs font-medium tracking-wide">
              {category.charAt(0).toUpperCase() + category.slice(1)}
            </div>
            {items.map((item) => (
              <a
                key={item.id}
                href={getStaticQuickstartHref(item, useHandoffLinks)}
                className="text-foreground-alt hover:bg-foreground/5 mx-2 flex items-center gap-3 rounded-lg px-3 py-1.5 no-underline transition-colors duration-200"
              >
                <div className="bg-foreground/10 text-foreground rounded-landing-control shadow-landing-tile flex size-9 shrink-0 items-center justify-center">
                  <item.icon className="icon-stroke-thin size-5" />
                </div>
                <div>
                  <div className="text-sm font-medium">{item.name}</div>
                  <div className="text-xs opacity-60">{item.description}</div>
                </div>
              </a>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

const GetStarted = ({ className, sessions }: GetStartedProps) => {
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

  if (isStatic) return <StaticGetStarted className={className} />

  return (
    <Command variant="landing" className={className}>
      <CommandInput
        ref={inputRef}
        variant="landing"
        placeholder="Where would you like to start? Type here to get started instantly."
        onKeyDown={handleKeyDown}
      />
      <CommandList variant="landing">
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
