import { createRoot } from 'react-dom/client'

import { hasInteracted } from '@s4wave/web/state/interaction.js'
import { AppLoadingScreen } from '@s4wave/app/loading/AppLoadingScreen.js'
import { PrerenderedApp } from './PrerenderedApp.js'

function Main() {
  const isReturningUser = hasInteracted()

  if (isReturningUser) {
    return <AppLoadingScreen />
  }
  return <PrerenderedApp />
}

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(<Main />)
}
