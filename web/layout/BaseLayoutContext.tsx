import React, { createContext, useContext } from 'react'
import { IJsonModel } from '@aptre/flex-layout'

// BaseLayoutContextValue defines the shape of our context
export interface BaseLayoutContextValue {
  // onModelChange is a hook to override the model after a change is made.
  // if undefined or null is returned, cancels the change entirely.
  onModelChange?: (before: IJsonModel, after: IJsonModel) => IJsonModel | null
}

export const BaseLayoutContext = createContext<
  BaseLayoutContextValue | undefined
>(undefined)

export interface BaseLayoutContextProviderProps extends BaseLayoutContextValue {
  children: React.ReactNode
}

export function BaseLayoutContextProvider({
  children,
  ...value
}: BaseLayoutContextProviderProps) {
  return (
    <BaseLayoutContext.Provider value={value}>
      {children}
    </BaseLayoutContext.Provider>
  )
}

export function useBaseLayoutContext(): BaseLayoutContextValue | undefined {
  return useContext(BaseLayoutContext)
}
