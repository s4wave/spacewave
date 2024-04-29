import React from 'react'

import { BldrComponent, DebugInfo, renderProto } from '@aptre/bldr-react'
import { retryWithAbort } from '@aptre/bldr'
import { EchoerClient } from '@go/github.com/aperturerobotics/starpc/echo/index.js'

import './example.css'
import { ExampleProps } from './example.pb.js'

// IExampleState contains state for Example.
interface IExampleState {
  message?: string
}

// ClassExample is an example of a class component implementing Example.
// This is no longer recommended (use function components).
export class Example extends BldrComponent<ExampleProps, IExampleState> {
  private echoHost?: EchoerClient

  constructor(props: ExampleProps) {
    super(props)
    this.state = {}
  }

  public componentDidMount() {
    this.echoHost = new EchoerClient(this.buildWebViewHostClient())
    retryWithAbort(this.abortController.signal, this.runEchoRpc.bind(this), {
      errorCb: (err) => {
        console.warn('example Echo failed', err)
      },
    })
  }

  private async runEchoRpc(): Promise<void> {
    const resp = await this.echoHost?.Echo({
      body: this.props.msg,
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
export default renderProto<ExampleProps>(
  ExampleProps,
  (props: ExampleProps) => <Example {...props} />,
)
