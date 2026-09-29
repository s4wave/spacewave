import {
  State,
  Task,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'

import type { IWorldState } from '../world/world-state.js'
import { watchForgeBlock } from './watch.js'

/** watchTask reads accepted Forge revisions of a Task until it completes. */
export function watchTask(
  world: IWorldState,
  key: string,
  signal?: AbortSignal,
): AsyncGenerator<Task> {
  return watchForgeBlock(
    world,
    key,
    {
      blockType: 'forge/task',
      decode: (data) => Task.fromBinary(data),
      isTerminal: (task) => task.taskState === State.TaskState_COMPLETE,
    },
    signal,
  )
}
