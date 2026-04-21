import type {
  FSCursor,
  FSCursorChangeCb,
  FSCursorOps,
} from '../../fs-cursor.js'
import type { FSCursorService } from '../rpc_srpc.pb.js'
import { FSCursorClient } from './fs-cursor-client.js'

// RpcFSCursor implements FSCursor on top of the FSCursorService.
// The first cursor returned from getProxyCursor manages the event stream.
export class RpcFSCursor implements FSCursor {
  private released = false
  private readonly signal: AbortSignal
  private readonly client: FSCursorService

  constructor(signal: AbortSignal, client: FSCursorService) {
    this.signal = signal
    this.client = client
  }

  // checkReleased checks if the fs cursor is currently released.
  checkReleased(): boolean {
    return this.released
  }

  // getProxyCursor builds the FSCursorClient and returns it as the proxy.
  async getProxyCursor(_signal?: AbortSignal): Promise<FSCursor | null> {
    const fsc = await FSCursorClient.build(this.client, this.signal)
    return fsc.rootCursor
  }

  // getCursorOps always returns null; delegates via getProxyCursor.
  async getCursorOps(_signal?: AbortSignal): Promise<FSCursorOps | null> {
    return null
  }

  // addChangeCb is not applicable to the root client FSCursor.
  addChangeCb(_cb: FSCursorChangeCb): void {}

  // release releases the filesystem cursor.
  release(): void {
    this.released = true
  }

  [Symbol.dispose](): void {
    this.release()
  }
}
