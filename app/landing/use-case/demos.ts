import type { SpacewaveAppProps } from '@s4wave/app/SpacewaveApp.js'

import { driveDemo } from './seeds/drive.js'
import { quickstartDemo } from './seeds/quickstart.js'

// UseCaseDemoSetup seeds one use-case demo in its ephemeral app.
export interface UseCaseDemoSetup {
  title: string
  setup: NonNullable<SpacewaveAppProps['setup']>
}

// USE_CASE_DEMOS maps each use-case page slug to the seed for its live demo.
// The debug scenarios open the same seeds as landing-<slug>.
export const USE_CASE_DEMOS = {
  drive: { title: 'Drive landing demo', setup: driveDemo },
  notes: { title: 'Notes landing demo', setup: quickstartDemo('notebook') },
  chat: { title: 'Chat landing demo', setup: quickstartDemo('chat') },
  devices: { title: 'Devices landing demo', setup: quickstartDemo('device') },
  plugins: { title: 'Plugins landing demo', setup: quickstartDemo('sql') },
} satisfies Record<string, UseCaseDemoSetup>

// DemoId names a use-case demo by its landing page slug.
export type DemoId = keyof typeof USE_CASE_DEMOS
