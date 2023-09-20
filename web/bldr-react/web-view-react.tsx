import React, { Suspense } from 'react'
import { WebViewErrorBoundary } from './web-view-error-boundary.js'

// LoadedReactComponentType is the type the loaded component should implement.
type LoadedReactComponentType = React.ComponentType<unknown>

// LoadedReactComponent is a lazy-loaded React component.
type LoadedReactComponent = React.LazyExoticComponent<LoadedReactComponentType>

// IReactComponentContainerProps are props for ReactComponentContainer.
export interface IReactComponentContainerProps {
  // scriptPath is the function component script path to render.
  scriptPath: string
  // componentProps is an optional props message to the component.
  componentProps?: Uint8Array
  // renderLoading renders the fallback when loading the content.
  renderLoading?: React.ReactNode
}

// ReactComponentContainer imports and initializes a ReactComponent script.
export function ReactComponentContainer(props: IReactComponentContainerProps) {
  const LoadedComponent: LoadedReactComponent = React.lazy(
    async (): Promise<{ default: LoadedReactComponentType }> =>
      import(props.scriptPath),
  )

  // TODO: how to set componentProps on reactComponent ?
  return (
    <WebViewErrorBoundary>
      <Suspense fallback={props.renderLoading ?? <div>Loading...</div>}>
        <LoadedComponent />
      </Suspense>
    </WebViewErrorBoundary>
  )
  /*
  <div
    style={{
      width: '100%',
      height: '100%',
      position: 'relative',
      overflow: 'hidden',
    }}
    ref={(ref) => this.update(this.functionComponent, ref || undefined)}
  />
    */
}
