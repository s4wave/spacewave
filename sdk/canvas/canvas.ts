import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { CanvasResourceServiceClient } from './canvas_srpc.pb.js'
import type {
  CanvasState,
  CanvasNode,
  CanvasEdge,
  HiddenGraphLink,
  UpdateCanvasResponse,
  WatchCanvasStateResponse,
} from './canvas.pb.js'

// ICanvasHandle contains the CanvasHandle interface.
export interface ICanvasHandle {
  // getState fetches the current canvas state from the server.
  getState(abortSignal?: AbortSignal): Promise<CanvasState>

  // updateNodes sets or updates nodes on the canvas.
  updateNodes(
    nodes: Record<string, CanvasNode>,
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse>

  // removeNodes removes nodes by ID.
  removeNodes(
    nodeIds: string[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse>

  // addEdges adds edges to the canvas.
  addEdges(
    edges: CanvasEdge[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse>

  // removeEdges removes edges by ID.
  removeEdges(
    edgeIds: string[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse>

  // addHiddenGraphLinks hides world graph links on the canvas.
  addHiddenGraphLinks(
    links: HiddenGraphLink[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse>

  // removeHiddenGraphLinks shows world graph links on the canvas.
  removeHiddenGraphLinks(
    links: HiddenGraphLink[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse>

  // update applies a batch update to the canvas.
  update(
    opts: {
      setNodes?: Record<string, CanvasNode>
      removeNodeIds?: string[]
      addEdges?: CanvasEdge[]
      removeEdgeIds?: string[]
      addHiddenGraphLinks?: HiddenGraphLink[]
      removeHiddenGraphLinks?: HiddenGraphLink[]
    },
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse>

  // watchState streams canvas state changes.
  watchState(abortSignal?: AbortSignal): AsyncIterable<CanvasState>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// CanvasHandle represents a handle to a canvas resource.
// Each instance maps 1:1 to a Go CanvasResource on the backend.
export class CanvasHandle extends Resource implements ICanvasHandle {
  private service: CanvasResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new CanvasResourceServiceClient(resourceRef.client)
  }

  // getState fetches the current canvas state from the server.
  public async getState(abortSignal?: AbortSignal): Promise<CanvasState> {
    const resp = await this.service.GetCanvasState({}, abortSignal)
    return resp.state ?? { nodes: {}, edges: [], hiddenGraphLinks: [] }
  }

  // updateNodes sets or updates nodes on the canvas.
  public async updateNodes(
    nodes: Record<string, CanvasNode>,
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse> {
    return this.service.UpdateCanvas({ setNodes: nodes }, abortSignal)
  }

  // removeNodes removes nodes by ID.
  public async removeNodes(
    nodeIds: string[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse> {
    return this.service.UpdateCanvas({ removeNodeIds: nodeIds }, abortSignal)
  }

  // addEdges adds edges to the canvas.
  public async addEdges(
    edges: CanvasEdge[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse> {
    return this.service.UpdateCanvas({ addEdges: edges }, abortSignal)
  }

  // removeEdges removes edges by ID.
  public async removeEdges(
    edgeIds: string[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse> {
    return this.service.UpdateCanvas({ removeEdgeIds: edgeIds }, abortSignal)
  }

  // addHiddenGraphLinks hides world graph links on the canvas.
  public async addHiddenGraphLinks(
    links: HiddenGraphLink[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse> {
    return this.service.UpdateCanvas(
      { addHiddenGraphLinks: links },
      abortSignal,
    )
  }

  // removeHiddenGraphLinks shows world graph links on the canvas.
  public async removeHiddenGraphLinks(
    links: HiddenGraphLink[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse> {
    return this.service.UpdateCanvas(
      { removeHiddenGraphLinks: links },
      abortSignal,
    )
  }

  // update applies a batch update to the canvas.
  public async update(
    opts: {
      setNodes?: Record<string, CanvasNode>
      removeNodeIds?: string[]
      addEdges?: CanvasEdge[]
      removeEdgeIds?: string[]
      addHiddenGraphLinks?: HiddenGraphLink[]
      removeHiddenGraphLinks?: HiddenGraphLink[]
    },
    abortSignal?: AbortSignal,
  ): Promise<UpdateCanvasResponse> {
    return this.service.UpdateCanvas(
      {
        setNodes: opts.setNodes ?? {},
        removeNodeIds: opts.removeNodeIds ?? [],
        addEdges: opts.addEdges ?? [],
        removeEdgeIds: opts.removeEdgeIds ?? [],
        addHiddenGraphLinks: opts.addHiddenGraphLinks ?? [],
        removeHiddenGraphLinks: opts.removeHiddenGraphLinks ?? [],
      },
      abortSignal,
    )
  }

  // watchState streams canvas state changes.
  public async *watchState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<CanvasState> {
    const stream = this.service.WatchCanvasState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchCanvasStateResponse>) {
      if (resp.state) {
        yield resp.state
      }
    }
  }
}
