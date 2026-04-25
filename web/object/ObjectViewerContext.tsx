import { createContext, useContext } from 'react'
import type { ObjectViewerComponent } from './object.js'

export interface ObjectViewerContextValue {
  visibleComponents: ObjectViewerComponent[]
  selectedComponent?: ObjectViewerComponent
  onSelectComponent: (component: ObjectViewerComponent) => void
}

const ObjectViewerContext = createContext<ObjectViewerContextValue | null>(null)

export const ObjectViewerProvider = ObjectViewerContext.Provider

// useObjectViewer returns the viewer context or null when outside a provider.
export function useObjectViewer(): ObjectViewerContextValue | null {
  return useContext(ObjectViewerContext)
}
