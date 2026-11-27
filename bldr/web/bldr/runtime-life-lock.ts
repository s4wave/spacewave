// A runtime life lock is a Web Lock that a dedicated runtime Worker takes at
// startup and holds until it dies. The browser releases a Worker's locks when
// the Worker ends, whether it crashed, ran out of memory, closed itself or was
// terminated, so a document that queues a request for the lock is granted it
// exactly when the Worker is gone, with no heartbeat. A watcher must request
// the lock only after the Worker reported holding it: an earlier request finds
// the lock free and takes it itself.

// RuntimeLifeLockReport is the one message a runtime Worker posts to the
// document that created it: whether it took its life lock. The document acks
// attaches only after this report, so a Worker that cannot take the lock still
// reports, and the document fails that term rather than run it unwatched.
export interface RuntimeLifeLockReport {
  runtimeLifeLock: string
  held: boolean
}

// buildRuntimeLifeLockName returns the life lock name of one host term, the
// generation of a runtime's dedicated Worker.
export function buildRuntimeLifeLockName(
  runtimeId: string,
  generation: string,
): string {
  return `bldr-runtime-life-${runtimeId}-${generation}`
}

// holdRuntimeLifeLock takes the life lock for the rest of this Worker's life,
// then reports to the document that created the Worker whether it holds it.
export function holdRuntimeLifeLock(
  scope: DedicatedWorkerGlobalScope,
  name: string,
): void {
  const report = (held: boolean) => {
    scope.postMessage({
      runtimeLifeLock: name,
      held,
    } satisfies RuntimeLifeLockReport)
  }
  if (!scope.navigator.locks) {
    report(false)
    return
  }
  scope.navigator.locks
    .request(name, () => {
      report(true)
      return new Promise<never>(() => {})
    })
    .catch((err: unknown) => {
      console.warn('runtime life lock unavailable', name, err)
      report(false)
    })
}

// watchRuntimeLifeLock calls onGone once the Worker holding the named life
// lock is gone. Aborting the signal withdraws the watch; onGone is not called
// after the abort.
export function watchRuntimeLifeLock(
  name: string,
  signal: AbortSignal,
  onGone: () => void,
): void {
  navigator.locks
    .request(name, { signal }, () => {
      if (!signal.aborted) {
        onGone()
      }
    })
    .catch(() => {})
}
