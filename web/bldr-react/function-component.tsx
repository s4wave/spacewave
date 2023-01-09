import React from 'react'
import { createRoot, RootOptions } from 'react-dom/client'
import { IBldrContext, BldrContext } from './bldr-context.js'
import { MessageDefinition } from 'starpc'

// FunctionComponent is a function that instantiates a sub-component.
// Returns a function to call when releasing the component.
export type FunctionComponent<Props = void> = (
  ctx: IBldrContext,
  parent: HTMLDivElement,
  props?: Props
) => () => void

// RenderFunc is a valid render function.
type RenderFunc = (
  props?: Uint8Array
) => React.ReactNode | JSX.Element | undefined

// renderProto wraps a render function with parsing a protobuf props object.
export function renderProto<T>(
  def: MessageDefinition<T>,
  render: (props: T) => React.ReactNode | JSX.Element | undefined
): RenderFunc {
  return (props?: Uint8Array) => {
    return render(def.decode(props || new Uint8Array(0)))
  }
}

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
