import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

// AppLoadingScreen renders the full-screen boot state for returning users.
// Wraps the new LoadingScreen primitive with the animated Spacewave logo.
export function AppLoadingScreen() {
  return (
    <LoadingScreen
      view={{
        state: 'loading',
        title: 'Spacewave',
        detail: 'Loading application...',
      }}
      logo={<AnimatedLogo followMouse={false} />}
    />
  )
}
