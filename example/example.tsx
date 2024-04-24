import React, { useState } from 'react'
import {
  renderProto,
  DebugInfo,
  DebugInfoDisplay,
  DebugInfoProvider,
  BldrDebug,
} from '@aptre/bldr-react'
import { retryWithAbort, isMac, isElectron } from '@aptre/bldr'

import { EchoerClient } from '@go/github.com/aperturerobotics/starpc/echo/index.js'
import { ExampleProps } from './example_pb.js'

import './example.css'
import { PlainMessage } from '@bufbuild/protobuf'
import { useWebViewHostServiceClient } from 'web/bldr-react/hooks.js'

// Example is an example of a functional react component accessing a host rpc.
const Example: React.FC<PlainMessage<ExampleProps>> = (props) => {
  const [message, setMessage] = useState<string | undefined>(undefined)

  useWebViewHostServiceClient<EchoerClient>(
    (c) => new EchoerClient(c),
    (host, abortSignal) => {
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
    },
  )

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
