// Root file for the Web Entrypoint.
// The Entrypoint loads & hands off control to Bldr.
import React from 'react'
import { createRoot } from 'react-dom/client'

import { App } from '../bldr-react'

document.addEventListener('DOMContentLoaded', () => {
  const container = document.getElementById('bldr-root')
  const root = createRoot(container!)
  root.render(<App />)
})
