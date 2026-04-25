import { Root } from '@s4wave/sdk/root'
import { LocalProvider } from '@s4wave/sdk/provider/local/local.js'
import { ProviderInfo } from '@s4wave/core/provider/provider.pb.js'
import { CreateAccountResponse } from '@s4wave/sdk/provider/local/local.pb.js'
import { GetSessionInfoResponse } from '@s4wave/sdk/session/session.pb'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'

import {
  createQuickstartSetupFromSession,
  createDrive,
} from '@s4wave/app/quickstart/create.js'

// testProvider tests provider and session functionality.
export async function testProvider(
  rootResource: Root,
  abortSignal: AbortSignal,
) {
  // Track resources for cleanup.
  const cleanupResources: Array<{ [Symbol.dispose](): void }> = []
  const cleanup: RegisterCleanup = (resource) => {
    if (resource) {
      cleanupResources.push(resource)
    }
    return resource
  }

  try {
    // lookup the local provider
    using localProvider = await rootResource.lookupProvider('local')
    const localProviderInfo = (await localProvider.getProviderInfo()) ?? {}
    console.log(
      'loaded local provider info',
      ProviderInfo.toJsonString(localProviderInfo),
    )

    // create a local provider account
    const lp = new LocalProvider(localProvider.resourceRef)
    const localProviderAccountResp = await lp.createAccount(abortSignal)
    console.log(
      'created local provider account',
      CreateAccountResponse.toJsonString(localProviderAccountResp),
    )

    // mount the session
    const localSession = cleanup(
      await rootResource.mountSession(
        {
          sessionRef: localProviderAccountResp.sessionListEntry?.sessionRef,
        },
        abortSignal,
      ),
    )
    const localSessionInfo = await localSession.getSessionInfo()
    console.log(
      'mounted local session',
      GetSessionInfoResponse.toJsonString(localSessionInfo),
    )

    // create the space
    const createSpaceResponse = await localSession.createSpace(
      {
        spaceName: 'E2E Test Space',
      },
      abortSignal,
    )
    console.log('created space')

    // mount the space and access world state
    const setup = await createQuickstartSetupFromSession({
      session: localSession,
      spaceResp: createSpaceResponse,
      abortSignal,
      cleanup,
    })
    console.log('mounted space and accessed world state')

    // create drive with demo content
    await createDrive(setup.spaceWorld, abortSignal)
    console.log('created drive with demo content')
  } finally {
    // Clean up all resources in reverse order.
    for (let i = cleanupResources.length - 1; i >= 0; i--) {
      cleanupResources[i][Symbol.dispose]()
    }
  }
}
