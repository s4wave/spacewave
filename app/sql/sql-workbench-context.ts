import { createContext } from 'react'

// SqlWorkbenchTargetDbContext carries the parent workbench database target into
// embedded SQL viewers without persisting duplicate target state on each tab.
export const SqlWorkbenchTargetDbContext = createContext('')
