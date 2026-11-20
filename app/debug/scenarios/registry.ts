import { USE_CASE_DEMOS } from '@s4wave/app/landing/use-case/demos.js'

import { canvasFiles, imageContext } from './canvas-files.js'
import { fileReferences } from './file-references.js'
import { objectBrowser } from './object-browser.js'
import type { AppScenario } from './ScenarioPage.js'

export const appScenarios: AppScenario[] = [
  { name: 'canvas-files', title: 'Canvas and Files', setup: canvasFiles },
  {
    name: 'image-context',
    title: 'Image and Space context',
    setup: imageContext,
  },
  {
    name: 'file-references',
    title: 'File and its references',
    setup: fileReferences,
  },
  {
    name: 'object-browser',
    title: 'Object browser',
    setup: objectBrowser,
  },
  ...Object.entries(USE_CASE_DEMOS).map(([slug, demo]) => ({
    name: `landing-${slug}`,
    ...demo,
  })),
]
