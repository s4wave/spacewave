import { ClientResourceRef } from '../resource/client.js'
import { Resource } from '../resource/resource.js'
import {
  StateAtomResourceService,
  StateAtomResourceServiceClient,
} from './state_srpc.pb.js'
import {
  GetStateResponse,
  WatchStateRequest,
  WatchStateResponse,
} from './state.pb.js'

// StateAtom provides access to a persisted state atom.
export class StateAtom extends Resource {
  // service is the state atom resource service
  private service: StateAtomResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new StateAtomResourceServiceClient(resourceRef.client)
  }

  // getState returns the current state.
  public async getState(abortSignal?: AbortSignal): Promise<GetStateResponse> {
    return await this.service.GetState({}, abortSignal)
  }

  // setState updates the state.
  public async setState(
    stateJson: string,
    abortSignal?: AbortSignal,
  ): Promise<bigint> {
    const resp = await this.service.SetState({ stateJson }, abortSignal)
    return resp.seqno ?? 0n
  }

  // watchState returns the stream of state changes for use with useWatchStateRpc.
  public watchState(
    req?: WatchStateRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchStateResponse> {
    return this.service.WatchState(req ?? {}, abortSignal)
  }
}
