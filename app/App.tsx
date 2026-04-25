import React from 'react'
import { isDesktop } from '@aptre/bldr'
import { AppShell } from './AppShell.js'
import { EditorShell } from './EditorShell.js'
import { AppAPI } from './AppAPI.js'

import { DebugBridgeProvider } from '@s4wave/web/debug/DebugBridgeProvider.js'

import './debug/spacewave-global.js'
import '@s4wave/web/style/app.css'

// App is the primary entrypoint for the web app..
export const App: React.FC = () => {
  return (
    <AppShell
      windowFrame={{
        title: 'Spacewave',
        topBar: { hidden: !isDesktop },
      }}
    >
      {import.meta.env?.DEV && <DebugBridgeProvider />}
      <AppAPI>
        <EditorShell />
      </AppAPI>
    </AppShell>
  )
}

// App will be loaded to a WebView when the plugin is loaded.
export default () => <App />
