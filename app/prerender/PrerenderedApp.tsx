import { HashRouter } from '@s4wave/web/router/HashRouter.js'
import { Landing } from '@s4wave/app/landing/Landing.js'

import '@s4wave/web/style/app.css'

export function PrerenderedApp() {
  return (
    <HashRouter>
      <div className="bg-background flex h-screen w-screen flex-col overflow-hidden">
        <Landing />
      </div>
    </HashRouter>
  )
}

export default PrerenderedApp
