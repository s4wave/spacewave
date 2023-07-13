import React from 'react'
import { createRoot, RootOptions } from 'react-dom/client'
import { IBldrContext, BldrContext } from './bldr-context.js'
import { MessageDefinition } from 'starpc'

// FunctionComponent is a function that instantiates a sub-component.
// Returns a function to call when releasing the component.
export type FunctionComponent = (
  ctx: IBldrContext,
  parent: HTMLDivElement,
  props?: Uint8Array,
) => () => void

// RenderFunc is a valid render function.
type RenderFunc = (
  props?: Uint8Array,
) => React.ReactNode | JSX.Element | undefined

// renderProto wraps a render function with parsing a protobuf props object.
export function renderProto<T>(
  def: MessageDefinition<T>,
  render: (props: T) => React.ReactNode | JSX.Element | undefined,
): RenderFunc {
  return (props?: Uint8Array) => {
    return render(def.decode(props || new Uint8Array(0)))
  }
}

// createFunctionComponent builds a FunctionComponent from a React render function.
export function createFunctionComponent(
  render: RenderFunc,
  rootOptions?: RootOptions,
): FunctionComponent {
  return (
    ctx: IBldrContext,
    parent: HTMLDivElement,
    props?: Uint8Array,
  ): (() => void) => {
    const root = createRoot(parent, rootOptions)
    root.render(
      <BldrContext.Provider value={ctx}>{render(props)}</BldrContext.Provider>,
    )
    return root.unmount.bind(root)
  }
}

// createProtoFunctionComponent builds a FunctionComponent from a React component with a protobuf props message.
export function createProtoFunctionComponent<T>(
  def: MessageDefinition<T>,
  render: (props: T) => React.ReactNode | JSX.Element | undefined,
  rootOptions?: RootOptions,
): FunctionComponent {
  return createFunctionComponent(renderProto(def, render), rootOptions)
}
