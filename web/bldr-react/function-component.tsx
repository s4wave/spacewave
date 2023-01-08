import React from 'react'
import { createRoot, RootOptions } from 'react-dom/client'
import { IBldrContext, BldrContext } from './bldr-context.js'

// FunctionComponent is a function that instantiates a sub-component.
// Returns a function to call when releasing the component.
export type FunctionComponent<Props = void> = (
  ctx: IBldrContext,
  parent: HTMLDivElement,
  props?: Props
) => () => void

// createFunctionComponent builds a FunctionComponent from a React component.
export function createFunctionComponent<Props = void>(
  render: (props?: Props) => React.ReactNode | JSX.Element | undefined,
  rootOptions?: RootOptions
): FunctionComponent<Props> {
  return (
    ctx: IBldrContext,
    parent: HTMLDivElement,
    props?: Props
  ): (() => void) => {
    const root = createRoot(parent, rootOptions)
    root.render(
      <BldrContext.Provider value={ctx}>{render(props)}</BldrContext.Provider>
    )
    return root.unmount.bind(root)
  }
}
