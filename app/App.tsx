import React from 'react'
import { createRoot } from 'react-dom/client'
import { Client as SRPCClient } from 'starpc'
import { isDesktop } from '@aptre/bldr'
import { Client as ResourceClient } from '@aptre/bldr-sdk/resource/client.js'
import { ResourceServiceClient } from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'

import { DebugBridgeProvider } from '@s4wave/web/debug/DebugBridgeProvider.js'

import { AppAPI, type AppAPIProps } from './AppAPI.js'
import { AppShell } from './AppShell.js'
import { openBoundSocketStream } from './bound-socket.js'
import { EditorShell } from './EditorShell.js'
import { FileDropGuard } from './FileDropGuard.js'

import './debug/spacewave-global.js'
import '@s4wave/web/style/app.css'

// AppProps selects the runtime connection. Omitting resourceClient keeps the
// Bldr WebView transport.
export type AppProps = Pick<AppAPIProps, 'resourceClient'>

// App is the primary entrypoint for the web app.
export const App: React.FC<AppProps> = (connection) => {
  return (
    <AppShell
      windowFrame={{
        title: 'Spacewave',
        topBar: { hidden: !isDesktop },
      }}
    >
      <FileDropGuard />
      {import.meta.env?.DEV && <DebugBridgeProvider />}
      <AppAPI {...connection}>
        <EditorShell />
      </AppAPI>
    </AppShell>
  )
}

// renderBoundApp renders the app into root without the Bldr runtime, over the
// Resource service a bound web listener serves at resourcePath.
export function renderBoundApp(root: HTMLElement, resourcePath: string) {
  const url = new URL(resourcePath, location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  const service = new ResourceServiceClient(
    new SRPCClient(openBoundSocketStream(url.href)),
  )
  const resourceClient = new ResourceClient(
    service,
    new AbortController().signal,
  )
  createRoot(root).render(<App resourceClient={resourceClient} />)
}

// App will be loaded to a WebView when the plugin is loaded.
export default () => <App />
