import superjson from 'superjson'
import type { Root } from '@s4wave/sdk/root'
import type { StateType } from '@s4wave/web/state/persist.js'
import { MemoryStorage, type AppEnvironment } from './environment.js'

interface SavedEnvironment {
  storage: Record<string, string>
  state: StateType
  path: string
  params: Record<string, string>
}

// persistAppEnvironment keeps each named frontend's shell state in its supplied
// installation. App IDs distinguish simultaneous views. Ordinary state atoms
// already use the Root's per-atom persistence through
// SpacewaveRuntimeProviders.
export async function persistAppEnvironment(
  root: Root,
  environment: AppEnvironment,
  signal: AbortSignal,
  onError: (error: unknown) => void,
): Promise<() => Promise<void>> {
  const storage = environment.storage
  if (!(storage instanceof MemoryStorage))
    throw new Error('App persistence requires app-owned storage')
  const resource = await root.accessStateAtom(
    { storeId: `app-environment/${encodeURIComponent(environment.id)}` },
    signal,
  )
  try {
    const { stateJson } = await resource.getState(signal)
    if (stateJson && stateJson !== '{}') {
      const saved = superjson.parse<SavedEnvironment>(stateJson)
      storage.restore(saved.storage)
      environment.rootAtom.set(saved.state)
      environment.navigation.setAppPath(saved.path, saved.params)
    }
  } catch (error) {
    resource.release()
    throw error
  }

  let pending: string | undefined
  let writing: Promise<void> | undefined
  let failure: unknown
  const drain = async () => {
    while (pending !== undefined) {
      const value = pending
      pending = undefined
      await resource.setState(value)
      failure = undefined
    }
  }
  const changed = () => {
    pending = superjson.stringify({
      storage: storage.snapshot(),
      state: environment.rootAtom.get(),
      ...environment.navigation.getAppNavigation(),
    } satisfies SavedEnvironment)
    if (writing) return
    writing = drain()
      .catch((error: unknown) => {
        failure = error
        onError(error)
      })
      .finally(() => {
        writing = undefined
        if (pending !== undefined) changed()
      })
  }
  const unsubscribe = [
    storage.subscribe(changed),
    environment.rootAtom.subscribe(changed),
    environment.navigation.subscribe(changed),
  ]
  return async () => {
    for (const stop of unsubscribe) stop()
    try {
      // Each pending write follows the previous snapshot. Teardown drains
      // that order.
      // eslint-disable-next-line react-doctor/async-await-in-loop
      while (writing) await writing
      if (failure) throw failure
    } finally {
      resource.release()
    }
  }
}
