// Root file for the Sandbox application.
import React from 'react'
import ReactDOM from 'react-dom'

import './index.css'

import { App } from './app'

ReactDOM.render(<App/>, document.getElementById('root'))

// https://www.snowpack.dev/concepts/hot-module-replacement
if (undefined /* [snowpack] import.meta.hot */) {
  undefined /* [snowpack] import.meta.hot */
    .accept()
}
