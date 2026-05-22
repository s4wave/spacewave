import { useMemo } from 'react'
import { WebView } from '@aptre/bldr-react'

import { BldrDeveloperStatusApp } from './BldrDeveloperStatusApp.js'
import './status.css'

// BldrDeveloperStatusStartup is a Bldr web startup surface for devtool status.
export default function BldrDeveloperStatusStartup() {
  const loading = useMemo(
    () => <div className="bldr-dev-status-loading">Connecting to Bldr</div>,
    [],
  )
  return (
    <WebView loading={loading} startupProgress>
      <BldrDeveloperStatusApp />
    </WebView>
  )
}
