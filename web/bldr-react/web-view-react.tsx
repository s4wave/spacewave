import React, { Suspense } from 'react'
import { WebViewErrorBoundary } from './web-view-error-boundary.js'
import type {
  LoadedProtoComponent,
  ProtoComponentType,
} from './react-component.js'

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
  const LoadedComponent: ProtoComponentType = React.lazy(
    async (): Promise<{ default: LoadedProtoComponent }> =>
      import(props.scriptPath),
  )

  return (
    <WebViewErrorBoundary>
      <Suspense fallback={props.renderLoading ?? <div>Loading...</div>}>
        <LoadedComponent componentProps={props.componentProps} />
      </Suspense>
    </WebViewErrorBoundary>
  )
}
