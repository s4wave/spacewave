import { createContext, use, ReactNode } from 'react'

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

// useStaticHref returns a crawlable path in static mode or a hash path in app mode.
export function useStaticHref(path: string): string {
  const isStatic = use(StaticContext)
  return isStatic ? path : `#${path}`
}
