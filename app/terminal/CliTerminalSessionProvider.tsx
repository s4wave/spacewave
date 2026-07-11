import {
  createContext,
  use,
  useEffect,
  useMemo,
  useRef,
  type ReactNode,
} from 'react'

import type { TerminalPaneConnector } from './TerminalPane.js'
import { CliTerminalSession } from './CliTerminalSession.js'

class CliTerminalSessionRegistry {
  readonly #sessions = new Map<number, CliTerminalSession>()

  get(sessionIndex: number, connect: TerminalPaneConnector) {
    const existing = this.#sessions.get(sessionIndex)
    if (existing && !existing.ended) return existing
    existing?.dispose()
    const session = new CliTerminalSession(connect)
    this.#sessions.set(sessionIndex, session)
    return session
  }

  dispose() {
    for (const session of this.#sessions.values()) {
      session.dispose()
    }
    this.#sessions.clear()
  }
}

const CliTerminalSessionContext =
  createContext<CliTerminalSessionRegistry | null>(null)

// CliTerminalSessionProvider owns browser CLI sessions above remountable panes.
export function CliTerminalSessionProvider({
  children,
}: {
  children: ReactNode
}) {
  const registryRef = useRef<CliTerminalSessionRegistry | null>(null)
  if (!registryRef.current) {
    registryRef.current = new CliTerminalSessionRegistry()
  }
  useEffect(() => {
    const registry = registryRef.current
    return () => registry?.dispose()
  }, [])
  return (
    <CliTerminalSessionContext.Provider value={registryRef.current}>
      {children}
    </CliTerminalSessionContext.Provider>
  )
}

// useCliTerminalSession returns the persistent connector for one session index.
export function useCliTerminalSession(
  sessionIndex: number,
  connect: TerminalPaneConnector,
): TerminalPaneConnector {
  const registry = use(CliTerminalSessionContext)
  if (!registry) {
    throw new Error('CliTerminalSessionProvider is required')
  }
  return useMemo(
    () => (frames, signal) =>
      registry.get(sessionIndex, connect).attach(frames, signal),
    [connect, registry, sessionIndex],
  )
}
