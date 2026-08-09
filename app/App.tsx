import { isDesktop } from '@aptre/bldr'

import { DebugBridgeProvider } from '@s4wave/web/debug/DebugBridgeProvider.js'
import '@s4wave/web/style/app.css'

import { AppAPI } from './AppAPI.js'
import { AppShell } from './AppShell.js'
import { EditorShell } from './EditorShell.js'
import { FileDropGuard } from './FileDropGuard.js'

import './debug/spacewave-global.js'

// App is the primary entrypoint for the web app.
export function App() {
  return (
    <AppShell
      windowFrame={{
        title: 'Spacewave',
        topBar: { hidden: !isDesktop },
      }}
    >
      <FileDropGuard />
      {import.meta.env?.DEV && <DebugBridgeProvider />}
      <AppAPI>
        <EditorShell />
      </AppAPI>
    </AppShell>
  )
}

export default App
