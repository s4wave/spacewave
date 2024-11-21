import React, { useMemo } from 'react'

import { WebView } from '@aptre/bldr-react'

// ExampleStartup is rendered in the root React app loaded before Bldr is loaded.
// It controls what to display while loading the wasm app.
export default function ExampleStartup() {
  const loading = useMemo(() => <div>Loading example web view...</div>, [])
  return <WebView loading={loading} placeholder={loading} />
}
