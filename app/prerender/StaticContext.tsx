import { createContext, use, type ReactNode } from 'react'

import { useSessionIndex } from '@s4wave/web/contexts/SessionIndexContext.js'
import { sessionPagePath } from '@s4wave/app/routes/session-page-path.js'

// StaticContext signals whether the app is in static prerender mode.
// When true, hooks that depend on the Go runtime return safe defaults.
const StaticContext = createContext(false)

// StaticProvider wraps children in static mode context.
export function StaticProvider({ children }: { children: ReactNode }) {
  return <StaticContext value={true}>{children}</StaticContext>
}

// useIsStaticMode returns whether the app is in static prerender mode.
export function useIsStaticMode(): boolean {
  return use(StaticContext)
}

// useStaticHref returns a crawlable public path or a hash path in the current session.
export function useStaticHref(path: string): string {
  const isStatic = use(StaticContext)
  const sessionIndex = useSessionIndex()
  return isStatic ? path : `#${sessionPagePath(path, sessionIndex)}`
}

// useAppHref returns a hash path that opens path in the running app. From a
// static page, following it boots the app at that path.
export function useAppHref(path: string): string {
  const isStatic = use(StaticContext)
  const sessionIndex = useSessionIndex()
  return `#${isStatic ? path : sessionPagePath(path, sessionIndex)}`
}
