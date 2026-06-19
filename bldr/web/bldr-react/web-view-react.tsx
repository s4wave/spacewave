import React, { Component, Suspense, useMemo, useState } from 'react'
import { WebViewErrorBoundary } from './web-view-error-boundary.js'
import type {
  LoadedProtoComponent,
  ProtoComponentType,
} from './react-component.js'
import { useMemoUint8Array } from './hooks.js'
import { loadWebViewScriptModule } from './web-view-module-loader.js'

interface InnerComponentProps {
  componentProps?: Uint8Array
  LoadedComponent: ProtoComponentType
  onReady?: () => void
}

class InnerComponent extends Component<InnerComponentProps> {
  componentDidMount() {
    this.props.onReady?.()
  }

  render() {
    const LoadedComponent = this.props.LoadedComponent
    return <LoadedComponent componentProps={this.props.componentProps} />
  }
}

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
  const [retryAttempt, setRetryAttempt] = useState(0)

  const LoadedComponent: ProtoComponentType = useMemo(() => {
    // React caches rejected lazy payloads, so recoverable load retries must
    // allocate a new lazy component even when the script path is unchanged.
    void retryAttempt
    return React.lazy(
      async (): Promise<{ default: LoadedProtoComponent }> =>
        loadWebViewScriptModule(props.scriptPath),
    )
  }, [props.scriptPath, retryAttempt])

  return (
    <WebViewErrorBoundary
      onRecoverableRetry={() => setRetryAttempt((attempt) => attempt + 1)}
    >
      <Suspense fallback={null}>
        <InnerComponent
          componentProps={componentProps ?? undefined}
          LoadedComponent={LoadedComponent}
          onReady={props.onReady}
        />
      </Suspense>
    </WebViewErrorBoundary>
  )
}
