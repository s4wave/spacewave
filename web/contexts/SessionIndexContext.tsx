import { createContext, use } from 'react'

// SessionIndexContext provides the session index (from /u/:sessionIndex) to
// child components. The context is set by AppSession and consumed by any
// component that needs the session index without parsing the URL.
export const SessionIndexContext = createContext<number>(0)

// useSessionIndex returns the current session index from context.
export function useSessionIndex(): number {
  return use(SessionIndexContext)
}
