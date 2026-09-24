import { SpacewaveApp } from '@s4wave/app/SpacewaveApp.js'

import { USE_CASE_DEMOS, type DemoId } from './demos.js'

const ephemeralStorage = { ephemeral: true } as const

// LiveDemoAppProps identifies one live demo run.
export interface LiveDemoAppProps {
  demo: DemoId
  appId: string
}

// LiveDemoApp runs a use-case demo as the ordinary app on a private in-memory
// World. Unmounting it releases the World and everything written to it.
export function LiveDemoApp({ demo, appId }: LiveDemoAppProps) {
  return (
    <SpacewaveApp
      appId={appId}
      suppliedStorage={ephemeralStorage}
      setup={USE_CASE_DEMOS[demo].setup}
    />
  )
}
