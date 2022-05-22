// Root file for the Sandbox application.
import React from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import '@fontsource/raleway/400.css'
import '@fontsource/open-sans/700.css'

import { App } from '../bldr-react'

const container = document.getElementById('root')
const root = createRoot(container!)
// <div className="app"></div>
root.render(<App />)

// https://www.snowpack.dev/concepts/hot-module-replacement
/* eslint:disable-next-line */
if (undefined /* [snowpack] import.meta.hot */) {
    // @ts-ignore: Object is possibly 'null'.
    undefined /* [snowpack] import.meta.hot */
        .accept()
}

/*
interface ImportMeta {
  readonly hot?: {
    accept: () => void
  }
}

if ((import.meta as ImportMeta).hot) {
  ;(import.meta as ImportMeta).hot?.accept()
}
*/
