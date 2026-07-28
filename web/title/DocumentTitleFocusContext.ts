import { createContext, use } from 'react'

// DocumentTitleFocusContext carries the focused ObjectLayout tab ID.
export const DocumentTitleFocusContext = createContext<string | null>(null)

// useDocumentTitleFocus returns the active ObjectLayout tab, when nested.
export function useDocumentTitleFocus(): string | null {
  return use(DocumentTitleFocusContext)
}
