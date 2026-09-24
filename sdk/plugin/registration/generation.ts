import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { ActivationHandler } from '../../../bldr/plugin/plugin_srpc.pb.js'
import {
  GenerationServiceClient,
  RegistrationServiceClient,
} from './registration_srpc.pb.js'
import type { PrepareRequest } from './registration.pb.js'

/**
 * prepareRegistrations opens the private registration scope of one worker
 * generation beneath the core root Resource. Registrations made through the
 * returned ref stay hidden until activateRegistrations admits them together;
 * releasing the ref hides the whole generation.
 */
export async function prepareRegistrations(
  core: ClientResourceRef,
  request: PrepareRequest,
  signal: AbortSignal,
): Promise<ClientResourceRef> {
  const prepared = await new RegistrationServiceClient(core.client).Prepare(
    request,
    signal,
  )
  if (!prepared.resourceId) {
    throw new Error('plugin registration scope returned no resource id')
  }
  return core.createRef(prepared.resourceId)
}

/**
 * activateRegistrations atomically replaces the plugin family's visible
 * registrations with those made in scope.
 */
export async function activateRegistrations(
  scope: ClientResourceRef,
  signal: AbortSignal,
): Promise<void> {
  await new GenerationServiceClient(scope.client).Activate({}, signal)
}

/**
 * createActivationHandler serves the scheduler's staged replacement contract.
 * Activate waits for ready, which resolves the prepared scope after every
 * startup registration succeeds, or undefined for a historical worker that
 * publishes nothing.
 */
export function createActivationHandler(
  ready: () => Promise<ClientResourceRef | undefined>,
  signal: AbortSignal,
): ActivationHandler {
  return {
    Check: async () => ({}),
    Activate: async (_request, abort) => {
      const scope = await ready()
      if (!scope) throw new Error('historical plugins cannot be activated')
      await activateRegistrations(scope, AbortSignal.any([signal, abort]))
      return {}
    },
  }
}
