import { IBldrContext } from './bldr-context.js'

// FunctionComponent is a function that instantiates a sub-component.
// Returns a function to call when releasing the component.
export type FunctionComponent = (
  ctx: IBldrContext,
  parent: HTMLDivElement,
  props?: Uint8Array,
) => () => void
