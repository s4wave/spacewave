import type { IWorldState } from '../world/world-state.js'

/** ForgeBlockCodec decodes one Forge object type and reports its terminal state. */
export interface ForgeBlockCodec<T> {
  /** blockType is the World block type of the watched object. */
  blockType: string
  /** decode parses the exact block bytes of one accepted revision. */
  decode(data: Uint8Array): T
  /** isTerminal reports that no later revision can change the outcome. */
  isTerminal(value: T): boolean
}

/** watchForgeBlock reads accepted revisions of a Forge object until it is terminal. */
export async function* watchForgeBlock<T>(
  world: IWorldState,
  key: string,
  codec: ForgeBlockCodec<T>,
  signal?: AbortSignal,
): AsyncGenerator<T> {
  // The object reference belongs to this subscription, including early return.
  using object = await world.getObject(key, signal)
  if (!object) {
    throw new Error(`Forge object ${key} is unavailable`)
  }

  // Decode the exact root before waiting on its successor revision.
  while (!signal?.aborted) {
    // eslint-disable-next-line react-doctor/async-await-in-loop -- This SDK cursor waits for the preceding revision; its reads cannot run concurrently.
    const root = await object.getRootRef(signal)
    let value: T
    {
      using cursor = await object.accessWorldState(root.rootRef, signal)
      const block = await cursor.unmarshal(
        { blockType: codec.blockType },
        signal,
      )
      if (!block.found || !block.data) {
        throw new Error(`Forge object ${key} has no state`)
      }
      value = codec.decode(block.data)
    }
    yield value
    if (codec.isTerminal(value)) {
      return
    }
    await object.waitRev((root.rev ?? 0n) + 1n, false, signal)
  }
}
