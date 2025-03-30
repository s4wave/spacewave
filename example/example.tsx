import React, { useState } from 'react'
import {
  renderProto,
  DebugInfo,
  DebugInfoDisplay,
  DebugInfoProvider,
  BldrDebug,
  useWebViewHostServiceClient,
} from '@aptre/bldr-react'
import {
  retryWithAbort,
  isMac,
  isElectron,
  isLinux,
  isWindows,
} from '@aptre/bldr'
import { Home, Settings, User } from 'lucide-react'

import { EchoerClient } from '@go/github.com/aperturerobotics/starpc/echo/index.js'
import { ExampleProps } from './example.pb.js'

import './example.css'

// Example is an example of a functional react component accessing a host rpc.
const Example: React.FC<ExampleProps> = (props) => {
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

  // Render the message along with some sample icons.
  return (
    <p className="example-message flex items-center gap-2">
      {message || 'Loading...'}
      <Home className="h-4 w-4" />
      <Settings className="h-4 w-4" />
      <User className="h-4 w-4" />
    </p>
  )
}

// ExampleDebug wraps Example with a DebugInfoProvider
const ExampleDebug: React.FC<ExampleProps> = (props) => {
  return (
    <DebugInfoProvider>
      <DebugInfoDisplay />
      <BldrDebug />
      <DebugInfo>
        isElectron: {JSON.stringify(isElectron)}
        <br />
        isMac: {JSON.stringify(isMac)}
        <br />
        isLinux: {JSON.stringify(isLinux)}
        <br />
        isWindows: {JSON.stringify(isWindows)}
      </DebugInfo>
      <Example {...props} />
    </DebugInfoProvider>
  )
}

export default renderProto(ExampleProps, (props: ExampleProps) => (
  <ExampleDebug {...props} />
))
