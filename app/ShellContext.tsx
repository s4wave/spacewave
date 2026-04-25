import { createContext, useContext, ReactNode } from 'react'
import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'

// ShellContextValue provides shell-level state to descendant components.
export interface ShellContextValue {
  isGridMode: boolean
}

const ShellContext = createContext<ShellContextValue | null>(null)

// useShell returns the shell context.
export function useShell(): ShellContextValue {
  const context = useContext(ShellContext)
  if (!context) {
    throw new Error('useShell must be used within a ShellProvider')
  }
  return context
}

// useIsGridMode returns whether the shell is in grid mode.
// Returns false in static prerender mode.
export function useIsGridMode(): boolean {
  const isStatic = useIsStaticMode()
  const context = useContext(ShellContext)
  if (isStatic) return false
  return context?.isGridMode ?? false
}

// ShellProviderProps are the props for ShellProvider.
export interface ShellProviderProps {
  isGridMode: boolean
  children: ReactNode
}

// ShellProvider provides shell-level state to descendant components.
export function ShellProvider({ isGridMode, children }: ShellProviderProps) {
  return (
    <ShellContext.Provider value={{ isGridMode }}>
      {children}
    </ShellContext.Provider>
  )
}
