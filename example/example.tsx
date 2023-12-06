import React, { useState } from 'react'
import {
  renderProto,
  useWebViewHostClient,
  DebugInfo,
  DebugInfoDisplay,
  DebugInfoProvider,
  BldrDebug,
} from '@aptre/bldr-react'
import { retryWithAbort, isMac, isElectron } from '@aptre/bldr'

import { EchoerClientImpl } from '@go/github.com/aperturerobotics/starpc/echo/index.js'
import { ExampleProps } from './example.pb.js'

import './example.css'

// Example is an example of a functional react component accessing a host rpc.
const Example: React.FC<ExampleProps> = (props: ExampleProps) => {
  const [message, setMessage] = useState<string | undefined>(undefined)

  useWebViewHostClient((client, abortSignal) => {
    const host = new EchoerClientImpl(client)
    retryWithAbort(
      abortSignal,
      async () => {
        const resp = await host.Echo({
          body: props.msg,
        })
        setMessage(resp?.body)
      },
      {
        errorCb: (err) => {
          console.warn('example Echo failed', err)
        },
      },
    )
  })

  return (
    <DebugInfoProvider>
      <DebugInfoDisplay />
      <BldrDebug />
      <DebugInfo>
        isElectron: {JSON.stringify(isElectron)}
        <br />
        isMac: {JSON.stringify(isMac)}
      </DebugInfo>
      <div className="example-message">{message || 'Loading...'}</div>
    </DebugInfoProvider>
  )
}

export default renderProto<ExampleProps>(
  ExampleProps,
  (props: ExampleProps) => <Example {...props} />,
)
