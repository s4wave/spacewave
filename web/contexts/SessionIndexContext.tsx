import { createContext, useContext } from 'react'

// SessionIndexContext provides the session index (from /u/:sessionIndex) to child components.
// Set by AppSession, consumed by any component that needs the session index without parsing the URL.
export const SessionIndexContext = createContext<number>(0)

// useSessionIndex returns the current session index from context.
export function useSessionIndex(): number {
  return useContext(SessionIndexContext)
}
