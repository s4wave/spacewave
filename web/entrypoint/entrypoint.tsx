// Root file for the Web Entrypoint.
// The Entrypoint loads & hands off control to Bldr.
import React from 'react'
import { createRoot } from 'react-dom/client'
import './entrypoint.css'
import '@fontsource/raleway/400.css'
import '@fontsource/open-sans/700.css'

import { App } from '../bldr-react'

const container = document.getElementById('root')
const root = createRoot(container!)
root.render(<App />)
