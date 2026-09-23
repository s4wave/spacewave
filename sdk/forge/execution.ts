import {
  Execution,
  State,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

import type { IWorldState } from '../world/world-state.js'

/** watchExecution reads accepted Forge revisions until completion or cancellation. */
export async function* watchExecution(
  world: IWorldState,
  key: string,
  signal?: AbortSignal,
): AsyncGenerator<Execution> {
  // The object reference belongs to this subscription, including early return.
  using object = await world.getObject(key, signal)
  if (!object) {
    throw new Error('Build execution is unavailable')
  }

  // Decode the exact root before waiting on its successor revision.
  while (!signal?.aborted) {
    // eslint-disable-next-line react-doctor/async-await-in-loop -- This SDK cursor waits for the preceding revision; its reads cannot run concurrently.
    const root = await object.getRootRef(signal)
    let execution: Execution
    {
      using cursor = await object.accessWorldState(root.rootRef, signal)
      const block = await cursor.unmarshal(
        { blockType: 'forge/execution' },
        signal,
      )
      if (!block.found || !block.data) {
        throw new Error('Build state is unavailable')
      }
      execution = Execution.fromBinary(block.data)
    }
    yield execution
    if (execution.executionState === State.ExecutionState_COMPLETE) {
      return
    }
    await object.waitRev((root.rev ?? 0n) + 1n, false, signal)
  }
}
