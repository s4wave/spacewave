import {
  createContext,
  use,
  useEffect,
  useMemo,
  useRef,
  type ReactNode,
} from 'react'

import { useShellTabs, useTabId } from '@s4wave/app/ShellTabContext.js'
import type { ShellTab } from '@s4wave/app/shell-tab.js'

import type { TerminalPaneConnector } from './TerminalPane.js'
import { CliTerminalSession } from './CliTerminalSession.js'

class CliTerminalSessionRegistry {
  readonly #sessions = new Map<
    string,
    { sessionIndex: number; session: CliTerminalSession }
  >()

  get(tabId: string, sessionIndex: number, connect: TerminalPaneConnector) {
    const existing = this.#sessions.get(tabId)
    if (existing?.sessionIndex === sessionIndex && !existing.session.ended) {
      return existing.session
    }
    existing?.session.dispose()
    const session = new CliTerminalSession(connect)
    this.#sessions.set(tabId, { sessionIndex, session })
    return session
  }

  retain(tabs: ShellTab[]) {
    const paths = new Map(tabs.map((tab) => [tab.id, tab.path]))
    for (const [tabId, { sessionIndex, session }] of this.#sessions) {
      if (paths.get(tabId) === `/u/${sessionIndex}/settings/cli/terminal`)
        continue
      session.dispose()
      this.#sessions.delete(tabId)
    }
  }

  dispose() {
    for (const { session } of this.#sessions.values()) {
      session.dispose()
    }
    this.#sessions.clear()
  }
}

const CliTerminalSessionContext =
  createContext<CliTerminalSessionRegistry | null>(null)

// CliTerminalSessionProvider retains each tab’s CLI stream across pane remounts.
// Closing the tab or navigating away releases its stream.
export function CliTerminalSessionProvider({
  children,
}: {
  children: ReactNode
}) {
  const { tabs } = useShellTabs()
  const registryRef = useRef<CliTerminalSessionRegistry | null>(null)
  if (!registryRef.current) {
    registryRef.current = new CliTerminalSessionRegistry()
  }
  useEffect(() => {
    registryRef.current?.retain(tabs)
  }, [tabs])
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

// useCliTerminalSession returns the persistent connector for the current shell tab.
export function useCliTerminalSession(
  sessionIndex: number,
  connect: TerminalPaneConnector,
): TerminalPaneConnector {
  const tabId = useTabId()
  const registry = use(CliTerminalSessionContext)
  if (!registry || !tabId) {
    throw new Error('A shell tab and CliTerminalSessionProvider are required')
  }
  return useMemo(
    () => (frames, signal) =>
      registry.get(tabId, sessionIndex, connect).attach(frames, signal),
    [connect, registry, sessionIndex, tabId],
  )
}
