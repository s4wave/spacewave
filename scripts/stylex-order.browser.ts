import { createElement } from 'react'
import { createRoot } from 'react-dom/client'

import { StylesheetLink } from '../bldr/web/bldr-react/StylesheetLink.js'
import { watchStylexOrder } from '../bldr/web/bldr/stylex-order.js'

declare global {
  var mountStylexSheet: (href: string) => Promise<void>
  var activateStylexOrder: () => void
}

// mountStylexSheet exercises the same load boundary that reveals a WebView.
globalThis.mountStylexSheet = (href) =>
  new Promise<void>((resolve) => {
    const host = document.createElement('div')
    document.body.append(host)
    createRoot(host).render(
      createElement(StylesheetLink, {
        id: href,
        href,
        onLoad: () => resolve(),
      }),
    )
  })
globalThis.activateStylexOrder = () => watchStylexOrder(document)
