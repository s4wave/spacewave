import React, { Suspense, useMemo } from 'react'
import { WebViewErrorBoundary } from './web-view-error-boundary.js'
import type {
  LoadedProtoComponent,
  ProtoComponentType,
} from './react-component.js'
import { useMemoUint8Array } from './hooks.js'

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
  const LoadedComponent: ProtoComponentType = useMemo(
    () =>
      React.lazy(
        async (): Promise<{ default: LoadedProtoComponent }> =>
          import(props.scriptPath),
      ),
    [props.scriptPath],
  )

  const componentProps = useMemoUint8Array(props.componentProps ?? null)
  const loadedComponent = useMemo(
    () => <LoadedComponent componentProps={componentProps ?? undefined} />,
    [LoadedComponent, componentProps],
  )

  return (
    <WebViewErrorBoundary>
      <Suspense fallback={props.renderLoading ?? <div>Loading...</div>}>
        {loadedComponent}
      </Suspense>
    </WebViewErrorBoundary>
  )
}
