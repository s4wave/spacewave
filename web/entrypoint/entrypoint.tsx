import React from 'react'
import { createRoot } from 'react-dom/client'
import { BldrRoot, IBldrRootProps } from '@aptre/bldr-react'
import { WebDocumentOptions } from '@aptre/bldr'
import { ReactComponentContainer } from 'web/bldr-react/web-view-react.js'

const webDocumentOpts: WebDocumentOptions = {}

// BLDR_RUNTIME_JS is an injected variable with the path to the runtime.js
declare const BLDR_RUNTIME_JS: string | undefined
if (typeof BLDR_RUNTIME_JS === 'string') {
  webDocumentOpts.runtimeWorkerPath = BLDR_RUNTIME_JS
}

// BLDR_SW_JS is an injected variable with the path to the sw.mjs
declare const BLDR_SW_JS: string | undefined
if (typeof BLDR_SW_JS === 'string') {
  webDocumentOpts.serviceWorkerPath = BLDR_SW_JS
}

const bldrRootProps: IBldrRootProps = { webDocumentOpts }

// BLDR_STARTUP_JS is an injected variable with the path to the startup js component
declare const BLDR_STARTUP_JS: string | undefined
if (typeof BLDR_STARTUP_JS === 'string') {
  bldrRootProps.disableRootWebView = true
  bldrRootProps.children = (
    <ReactComponentContainer scriptPath={BLDR_STARTUP_JS} />
  )
}

// initialize react and Bldr
document.addEventListener('DOMContentLoaded', () => {
  const container = document.getElementById('bldr-root')
  const root = createRoot(container!)
  root.render(<BldrRoot {...bldrRootProps} />)
})
