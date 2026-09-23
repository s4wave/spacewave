import type {
  AppSetupContext,
  AppSetupResult,
} from '@s4wave/app/SpacewaveApp.js'
import {
  buildQuickstartSpaceRoutePath,
  createQuickstartSetup,
  type QuickstartSetup,
} from '@s4wave/app/quickstart/create.js'
import type { QuickstartSpaceCreateId } from '@s4wave/app/quickstart/options.js'

// quickstartDemo returns a demo setup that runs one Quickstart and opens its
// initial object.
export function quickstartDemo(
  quickstartId: QuickstartSpaceCreateId,
): (context: AppSetupContext) => Promise<AppSetupResult> {
  return async (context) => {
    const setup = await runQuickstart(context, quickstartId)
    return {
      path: quickstartDemoPath(setup, quickstartId),
      debug: { setup },
    }
  }
}

// runQuickstart creates and seeds a Quickstart Space through the same path a
// new user runs, reporting its progress to the demo's loading screen.
export function runQuickstart(
  { root, signal, cleanup, reportProgress }: AppSetupContext,
  quickstartId: QuickstartSpaceCreateId,
): Promise<QuickstartSetup> {
  return createQuickstartSetup(root, quickstartId, signal, cleanup, (state) =>
    reportProgress(state.detail),
  )
}

// quickstartDemoPath returns the app path that opens a seeded demo Space at
// objectKey, or at the Quickstart's initial object when objectKey is empty.
export function quickstartDemoPath(
  setup: QuickstartSetup,
  quickstartId: QuickstartSpaceCreateId,
  objectKey?: string,
): string {
  const spaceId = setup.spaceResp.sharedObjectRef?.providerResourceRef?.id
  if (!spaceId || !setup.sessionIndex) {
    throw new Error('Demo setup did not return a Session and Space')
  }
  return buildQuickstartSpaceRoutePath(
    `/u/${setup.sessionIndex}/so/${spaceId}`,
    quickstartId,
    objectKey,
  )
}
