import { LoadingScreen as BaseLoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

// LoadingScreen is the full-screen boot surface used while a quickstart
// initializes a session. Drives the LoadingScreen primitive with a dynamic
// quickstart-id-driven view.
export function LoadingScreen({ quickstartId }: { quickstartId: string }) {
  return (
    <BaseLoadingScreen
      view={{
        state: 'active',
        title: 'Initializing Spacewave',
        detail: `Setting up ${quickstartId}`,
      }}
      logo={<AnimatedLogo followMouse={false} />}
    />
  )
}
