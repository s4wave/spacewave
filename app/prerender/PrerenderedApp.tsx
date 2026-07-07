import { HashRouter } from '@s4wave/web/router/HashRouter.js'
import { Landing } from '@s4wave/app/landing/Landing.js'

import { ROOT_LANDING_SHELL_CLASS } from './root-landing-shell.js'

import '@s4wave/web/style/app.css'

export function PrerenderedApp() {
  return (
    <HashRouter>
      <div className={ROOT_LANDING_SHELL_CLASS}>
        <Landing />
      </div>
    </HashRouter>
  )
}

export default PrerenderedApp
