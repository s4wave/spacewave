import React from 'react'
import { useMemo, useRef, useCallback, useEffect } from 'react'
import { LuUser } from 'react-icons/lu'

import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import {
  getQuickstartPath,
  VISIBLE_QUICKSTART_OPTIONS,
} from '../quickstart/options.js'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@s4wave/web/ui/command.js'
import { useIsStaticMode } from '../prerender/StaticContext.js'

const COMMAND_ITEMS = [...VISIBLE_QUICKSTART_OPTIONS]

const CATEGORIES = [...new Set(COMMAND_ITEMS.map((item) => item.category))]

const GetStartedItem = ({ item }: { item: (typeof COMMAND_ITEMS)[number] }) => {
  const navigate = useNavigate()
  const handleClick = useCallback(() => {
    const path = getQuickstartPath(item)
    navigate({ path })
  }, [item, navigate])

  return (
    <CommandItem
      key={item.id}
      className="text-foreground-alt flex cursor-pointer items-center gap-3 px-4 py-1.5"
      onSelect={handleClick}
    >
      <div className="bg-foreground/5 flex h-9 w-9 items-center justify-center rounded-md transition-colors">
        <item.icon className="h-5 w-5 stroke-[1.5]" />
      </div>
      <div>
        <div className="text-sm font-medium">{item.name}</div>
        <div className="text-xs opacity-70">{item.description}</div>
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
    (metadata?.providerId === 'spacewave' ? 'Cloud'
    : metadata?.providerId === 'local' ? 'Local'
    : 'Account')
  const subtitle =
    metadata?.cloudEntityId && metadata.cloudEntityId !== accountName ?
      `${providerLabel} · ${metadata.cloudEntityId}`
    : providerLabel

  const handleClick = useCallback(() => {
    navigate({ path: '/u/' + session.sessionIndex + '/' })
  }, [navigate, session.sessionIndex])

  return (
    <CommandItem
      key={session.sessionIndex}
      className="text-foreground-alt flex cursor-pointer items-center gap-3 px-4 py-1.5"
      onSelect={handleClick}
    >
      <div className="bg-foreground/5 flex h-9 w-9 items-center justify-center rounded-md transition-colors">
        <LuUser className="h-5 w-5 stroke-[1.5]" />
      </div>
      <div>
        <div className="text-sm font-medium">Account: {accountName}</div>
        <div className="text-xs opacity-70">{subtitle}</div>
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
  const itemsByCategory = useMemo(() => {
    return CATEGORIES.map((category) => ({
      category,
      items: COMMAND_ITEMS.filter((item) => item.category === category),
    }))
  }, [])

  return (
    <div
      className={cn(
        'border-foreground/20 bg-background-get-started relative flex min-h-[200px] flex-1 flex-col overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm',
        'tall:max-h-[min(55vh,650px)] tall:flex-initial @lg:max-h-[min(38vh,450px)] @2xl:max-h-[min(50vh,550px)]',
        className,
      )}
    >
      <div className="placeholder:text-foreground/70 border-foreground/10 flex items-center gap-2 border-b px-3 py-2.5">
        <span className="text-foreground/70 text-sm">
          Where would you like to start? Type here to get started instantly.
        </span>
      </div>
      <div className="bg-background-get-started min-h-0 flex-1 overflow-y-auto pb-2">
        {itemsByCategory.map(({ category, items }) => (
          <div key={category} className="mb-0 py-0">
            <div className="text-foreground/50 px-4 py-1.5 text-xs font-medium">
              {category.charAt(0).toUpperCase() + category.slice(1)}
            </div>
            {items.map((item) => (
              <a
                key={item.id}
                href={getQuickstartPath(item)}
                className="text-foreground-alt flex items-center gap-3 px-4 py-1.5 no-underline"
              >
                <div className="bg-foreground/5 flex h-9 w-9 items-center justify-center rounded-md">
                  <item.icon className="h-5 w-5 stroke-[1.5]" />
                </div>
                <div>
                  <div className="text-sm font-medium">{item.name}</div>
                  <div className="text-xs opacity-70">{item.description}</div>
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
  const inputRef = useRef<HTMLInputElement>(null)
  const itemsByCategory = useMemo(() => {
    return CATEGORIES.map((category) => ({
      category,
      items: COMMAND_ITEMS.filter((item) => item.category === category),
    }))
  }, [])

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
    <Command
      className={cn(
        'border-foreground/20 bg-background-get-started relative flex min-h-[200px] flex-1 flex-col overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm',
        'tall:max-h-[min(55vh,650px)] tall:flex-initial @lg:max-h-[min(38vh,450px)] @2xl:max-h-[min(50vh,550px)]',
        className,
      )}
    >
      <CommandInput
        ref={inputRef}
        className="placeholder:text-foreground/70 border-foreground/10 border-b"
        placeholder="Where would you like to start? Type here to get started instantly."
        onKeyDown={handleKeyDown}
      />
      <CommandList className="bg-background-get-started min-h-0 flex-1 overflow-y-auto pb-2">
        <CommandEmpty>No templates found.</CommandEmpty>
        {sessions && sessions.length > 0 && (
          <CommandGroup heading="Sessions" className="mb-0 py-0">
            {sessions.map((session) => (
              <SessionItem key={session.sessionIndex} session={session} />
            ))}
          </CommandGroup>
        )}
        {itemsByCategory.map(({ category, items }) => (
          <CommandGroup
            key={category}
            heading={category.charAt(0).toUpperCase() + category.slice(1)}
            className="mb-0 py-0"
          >
            {items.map((item) => (
              <GetStartedItem key={item.id} item={item} />
            ))}
          </CommandGroup>
        ))}
      </CommandList>
    </Command>
  )
}

export default GetStarted
