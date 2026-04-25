import { useCallback } from 'react'

import { useCommand } from '@s4wave/web/command/useCommand.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'

// triggerDownload creates a temporary anchor to start a file download.
function triggerDownload(path: string, filename: string) {
  const a = document.createElement('a')
  a.href = pluginPathPrefix + path
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

// DebugCommands registers debug/profiling commands with the command palette.
// Only rendered in debug builds (gated by BLDR_DEBUG in EditorShell).
export function DebugCommands() {
  useCommand({
    commandId: 'spacewave.debug.trace',
    label: 'Download Trace (30s)',
    description: 'Capture a 30-second Go runtime trace',
    menuPath: 'Tools/Debug/Download Trace',
    menuGroup: 50,
    menuOrder: 1,
    handler: useCallback(() => {
      triggerDownload('/debugz/trace?seconds=30', 'trace.out')
    }, []),
  })

  useCommand({
    commandId: 'spacewave.debug.goroutines',
    label: 'Copy Goroutines to Clipboard',
    description: 'Copy goroutine dump to clipboard',
    menuPath: 'Tools/Debug/Copy Goroutines',
    menuGroup: 50,
    menuOrder: 2,
    handler: useCallback(() => {
      void fetch(pluginPathPrefix + '/debugz/pprof/goroutine?debug=1')
        .then((r) => {
          if (!r.ok) throw new Error(r.statusText)
          return r.text()
        })
        .then((text) => navigator.clipboard.writeText(text))
        .catch((err) => console.error('Failed to copy goroutines:', err))
    }, []),
  })

  useCommand({
    commandId: 'spacewave.debug.heap',
    label: 'Download Heap Profile',
    description: 'Download heap memory profile',
    menuPath: 'Tools/Debug/Download Heap Profile',
    menuGroup: 50,
    menuOrder: 3,
    handler: useCallback(() => {
      triggerDownload('/debugz/pprof/heap', 'heap.pb.gz')
    }, []),
  })

  useCommand({
    commandId: 'spacewave.debug.cpu',
    label: 'Download CPU Profile (30s)',
    description: 'Capture a 30-second CPU profile',
    menuPath: 'Tools/Debug/Download CPU Profile',
    menuGroup: 50,
    menuOrder: 4,
    handler: useCallback(() => {
      triggerDownload('/debugz/pprof/profile?seconds=30', 'cpu.pb.gz')
    }, []),
  })

  return null
}
