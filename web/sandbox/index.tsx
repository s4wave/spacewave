// Root file for the Sandbox application.
import React from 'react'
import ReactDOM from 'react-dom'
import './index.css'
import '@fontsource/raleway/400.css'
import '@fontsource/open-sans/700.css'

import { App } from '../bldr-react'

// render the root of the app.
ReactDOM.render(
  <div className="app">
    <App />
  </div>,
  document.getElementById('root')
)

// https://www.snowpack.dev/concepts/hot-module-replacement
if (undefined /* [snowpack] import.meta.hot */) {
  undefined /* [snowpack] import.meta.hot */
    .accept()
}
