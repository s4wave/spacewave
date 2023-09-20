import React from 'react'
import { createRoot, RootOptions } from 'react-dom/client'
import { IBldrContext, BldrContext } from './bldr-context.js'
import { MessageDefinition } from 'starpc'
import { ProtoRenderFunc, renderProto } from './react-component.js'

// FunctionComponent is a function that instantiates a sub-component.
// Returns a function to call when releasing the component.
export type FunctionComponent = (
  ctx: IBldrContext,
  parent: HTMLDivElement,
  props?: Uint8Array,
) => () => void

// createReactFunctionComponent builds a FunctionComponent from a React render function.
// NOTE: not recommended: use ReactComponent instead.
export function createReactFunctionComponent(
  render: ProtoRenderFunc,
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

// createReactProtoFunctionComponent builds a FunctionComponent from a React component with a protobuf props message.
// NOTE: not recommended: use ReactComponent instead.
export function createReactProtoFunctionComponent<T>(
  def: MessageDefinition<T>,
  render: (props: T) => React.ReactNode | JSX.Element | undefined,
  rootOptions?: RootOptions,
): FunctionComponent {
  return createReactFunctionComponent(renderProto(def, render), rootOptions)
}
