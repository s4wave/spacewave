import React, { Suspense, useMemo, useEffect } from 'react'
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
  // onReady is called when the component is ready
  onReady?: () => void
}

// ReactComponentContainer imports and initializes a ReactComponent script.
export function ReactComponentContainer(props: IReactComponentContainerProps) {
  const componentProps = useMemoUint8Array(props.componentProps ?? null)

  const LoadedComponent: ProtoComponentType = useMemo(
    () =>
      React.lazy(
        async (): Promise<{ default: LoadedProtoComponent }> =>
          import(props.scriptPath),
      ),
    [props.scriptPath],
  )

  const InnerComponent = useMemo(
    () =>
      ({
        componentProps,
        onReady,
      }: {
        componentProps?: Uint8Array
        onReady?: () => void
      }) => {
        useEffect(() => {
          if (onReady) {
            onReady()
          }
        }, [onReady])

        return <LoadedComponent componentProps={componentProps} />
      },
    [LoadedComponent],
  )

  return (
    <WebViewErrorBoundary>
      <Suspense fallback={null}>
        <InnerComponent
          componentProps={componentProps ?? undefined}
          onReady={props.onReady}
        />
      </Suspense>
    </WebViewErrorBoundary>
  )
}
