import React, { useEffect } from 'react'
import { Toaster, toast } from 'sonner'

declare global {
  interface Window {
    __downstreamE2E?: {
      ready: boolean
      toastText?: string
      resources?: string[]
    }
  }
}

const toastText = 'Downstream Sonner loaded through Bldr'

export default function DownstreamApp() {
  useEffect(() => {
    window.__downstreamE2E = {
      ready: false,
      toastText,
      resources: [],
    }
    toast(toastText)
    window.__downstreamE2E = {
      ready: true,
      toastText,
      resources: performance
        .getEntriesByType('resource')
        .map((entry) => entry.name),
    }
  }, [])

  return (
    <main
      style={{
        fontFamily: 'system-ui, sans-serif',
        padding: 24,
      }}
    >
      <Toaster />
      <h1>Downstream GoScript E2E</h1>
      <p data-testid="downstream-ready">{toastText}</p>
    </main>
  )
}
