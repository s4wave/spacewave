import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import type { IWorldState } from '../world/world-state.js'
import {
  CreateFlowgraphOp,
  type FlowgraphSnapshot,
  type UpdateFlowgraphRequest,
} from './flowgraph.pb.js'
import { FlowgraphResourceServiceClient } from './flowgraph_srpc.pb.js'

/** FlowgraphTypeID is the World type of an authored Flowgraph. */
export const FlowgraphTypeID = 'flowgraph'

/** CreateFlowgraphOpID selects the independently addressed creation operation. */
export const CreateFlowgraphOpID = 'flowgraph/create'

/** createFlowgraph creates the body and type edge in the supplied transaction. */
export async function createFlowgraph(
  world: IWorldState,
  request: CreateFlowgraphOp,
  abortSignal?: AbortSignal,
): Promise<void> {
  await world.applyWorldOp(
    CreateFlowgraphOpID,
    CreateFlowgraphOp.toBinary(request),
    '',
    abortSignal,
  )
}

/** FlowgraphHandle reads, watches, and edits one graph through its Resource. */
export class FlowgraphHandle extends Resource {
  private readonly service: FlowgraphResourceServiceClient

  /** constructor retains the supplied Resource reference until release. */
  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new FlowgraphResourceServiceClient(resourceRef.client)
  }

  /** get reads the graph and its placement edges at one observed revision. */
  async get(abortSignal?: AbortSignal): Promise<FlowgraphSnapshot> {
    const response = await this.service.GetFlowgraph({}, abortSignal)
    if (!response.snapshot)
      throw new Error('Flowgraph read returned no snapshot')
    return response.snapshot
  }

  /** update commits one authored change and returns its committed revision. */
  async update(
    request: UpdateFlowgraphRequest,
    abortSignal?: AbortSignal,
  ): Promise<FlowgraphSnapshot> {
    const response = await this.service.UpdateFlowgraph(request, abortSignal)
    if (!response.snapshot)
      throw new Error('Flowgraph edit returned no snapshot')
    return response.snapshot
  }

  /** watch pushes committed graph changes until canceled or released. */
  async *watch(abortSignal?: AbortSignal): AsyncIterable<FlowgraphSnapshot> {
    const stream = this.service.WatchFlowgraph({}, abortSignal)
    for await (const response of stream) {
      if (response.snapshot) yield response.snapshot
    }
  }
}
