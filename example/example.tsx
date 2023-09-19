import React from 'react'

import {
  // createFunctionComponent,
  BldrComponent,
  DebugInfo,
} from '@aptre/bldr-react'
import { retryWithAbort } from '@aptre/bldr'
import { EchoerClientImpl } from '@go/github.com/aperturerobotics/starpc/echo/index.js'

import './example.css'

// IExampleState contains state for Example.
interface IExampleState {
  message?: string
}

class Example extends BldrComponent<{}, IExampleState> {
  // echoHost is the echo service running on the plugin host.
  private echoHost?: EchoerClientImpl

  constructor(props: {}) {
    super(props)
    this.state = {}
  }

  public componentDidMount() {
    this.echoHost = new EchoerClientImpl(this.buildWebViewHostClient())
    retryWithAbort(this.abortController.signal, this.runEchoRpc.bind(this), {
      errorCb: (err) => {
        console.warn('example Echo failed', err)
      },
    })
  }

  // runEchoRpc runs the echo rpc and updates the state.
  private async runEchoRpc(): Promise<void> {
    const resp = await this.echoHost?.Echo({
      body: 'Hello from TypeScript via RPC round-trip to the plugin!',
    })
    this.setState({ message: resp?.body })
  }

  public render() {
    return (
      <>
        <DebugInfo>TestDebugInfo</DebugInfo>
        <div className="example-message">
          {this.state.message || 'Loading...'}
        </div>
      </>
    )
  }
}

// Example will be constructed when the component is loaded.
// export default createFunctionComponent(() => <Example />)
export default () => <Example />
