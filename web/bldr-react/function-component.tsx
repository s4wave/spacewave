import React from 'react'
import { createRoot, RootOptions } from 'react-dom/client'
import { IBldrContext, BldrContext } from './bldr-context.js'

// FunctionComponent is a function that instantiates a sub-component.
// Returns a function to call when releasing the component.
export type FunctionComponent = (
  ctx: IBldrContext,
  parent: HTMLDivElement
) => () => void

// createFunctionComponent builds a FunctionComponent from a React component.
export function createFunctionComponent(
  children: React.ReactNode,
  rootOptions?: RootOptions
): FunctionComponent {
  return (ctx: IBldrContext, parent: HTMLDivElement): (() => void) => {
    const root = createRoot(parent, rootOptions)
    root.render(
      <BldrContext.Provider value={ctx}>{children}</BldrContext.Provider>
    )
    return root.unmount.bind(root)
  }
}
