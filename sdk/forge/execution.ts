import {
  Execution,
  State,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

import type { IWorldState } from '../world/world-state.js'
import { watchForgeBlock } from './watch.js'

/** watchExecution reads accepted Forge revisions until completion or cancellation. */
export function watchExecution(
  world: IWorldState,
  key: string,
  signal?: AbortSignal,
): AsyncGenerator<Execution> {
  return watchForgeBlock(
    world,
    key,
    {
      blockType: 'forge/execution',
      decode: (data) => Execution.fromBinary(data),
      isTerminal: (execution) =>
        execution.executionState === State.ExecutionState_COMPLETE,
    },
    signal,
  )
}
