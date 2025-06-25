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

import bldrLogo from '../doc/img/bldr-logo.png'

// Import file which imports .css file
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
    <div className="example-container">
      {isMac && isElectron && (
        <div className="title-bar">
          <div className="title-bar-controls">
            <div className="title-bar-button close"></div>
            <div className="title-bar-button minimize"></div>
            <div className="title-bar-button maximize"></div>
          </div>
          <div className="title-bar-title">Example App</div>
        </div>
      )}
      <div className="example-message">
        {message || 'Loading...'}
        <Home className="h-4 w-4" />
        <Settings className="h-4 w-4" />
        <User className="h-4 w-4" />
        <img src={bldrLogo} width={256} />
      </div>
    </div>
  )
}

// ExampleDebug wraps Example with a DebugInfoProvider
const ExampleDebug: React.FC<ExampleProps> = (props) => {
  return (
    <DebugInfoProvider>
      <DebugInfoDisplay style={{ marginTop: '40px' }} />
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
