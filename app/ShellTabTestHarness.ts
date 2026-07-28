import {
  BROWSER_SHELL_TABS_STORAGE_KEY,
  resetBrowserShellTabsStoreForTests,
  type BrowserShellTabRecord,
} from './BrowserShellTabsStore.js'

export interface ShellTabSeed {
  id: string
  path: string
  name?: string
  customName?: string
}

export function seedShellTabs(records: ShellTabSeed[], epoch = 0): void {
  localStorage.setItem(
    BROWSER_SHELL_TABS_STORAGE_KEY,
    JSON.stringify({
      schemaVersion: 1,
      epoch,
      revision: 0,
      records: records.map((record, index) => ({
        ...record,
        name: record.name ?? (record.path === '/' ? 'Home' : record.path),
        creationSequence: index + 1,
      })),
    }),
  )
  resetBrowserShellTabsStoreForTests()
}

export interface ShellTabTestBrowser {
  (): void
  blockNextMutation: () => Promise<void>
  releaseBlockedMutation: () => void
}

export function readShellTabsSnapshot(): {
  schemaVersion: number
  epoch: number
  revision: number
  records: BrowserShellTabRecord[]
} {
  const raw = localStorage.getItem(BROWSER_SHELL_TABS_STORAGE_KEY)
  if (!raw) {
    return { schemaVersion: 1, epoch: 0, revision: 0, records: [] }
  }
  return JSON.parse(raw) as {
    schemaVersion: number
    epoch: number
    revision: number
    records: BrowserShellTabRecord[]
  }
}

export function installShellTabTestBrowser(): ShellTabTestBrowser {
  const previousLocks = Object.getOwnPropertyDescriptor(navigator, 'locks')
  const queues = new Map<string, Promise<void>>()
  let disposed = false
  let blockNext = false
  let blockedReady: (() => void) | undefined
  let releaseBlocked: (() => void) | undefined
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: (
        name: string,
        _options: unknown,
        callback: (lock: object) => unknown,
      ) => {
        const previous = queues.get(name)
        let release!: () => void
        const current = new Promise<void>((resolve) => {
          release = resolve
        })
        queues.set(name, current)
        const invoke = (): unknown => {
          if (blockNext) {
            blockNext = false
            blockedReady?.()
            blockedReady = undefined
            return new Promise<void>((resolve) => {
              releaseBlocked = resolve
            }).then(() => invoke())
          }
          if (disposed)
            throw new Error('The test Web Locks manager is disposed.')
          return callback({})
        }
        let result: unknown
        try {
          result = previous ? previous.then(invoke) : invoke()
        } catch (error) {
          result = Promise.reject(error)
        }
        return Promise.resolve(result).then(
          (value) => {
            release()
            if (queues.get(name) === current) queues.delete(name)
            return value
          },
          (error: unknown) => {
            release()
            if (queues.get(name) === current) queues.delete(name)
            throw error
          },
        )
      },
    },
  })
  resetBrowserShellTabsStoreForTests()

  const restore = (() => {
    disposed = true
    releaseBlocked?.()
    queues.clear()
    if (previousLocks) {
      Object.defineProperty(navigator, 'locks', previousLocks)
    } else {
      delete (navigator as { locks?: unknown }).locks
    }
    resetBrowserShellTabsStoreForTests()
  }) as ShellTabTestBrowser
  restore.blockNextMutation = () => {
    blockNext = true
    return new Promise<void>((resolve) => {
      blockedReady = resolve
    })
  }
  restore.releaseBlockedMutation = () => {
    releaseBlocked?.()
    releaseBlocked = undefined
  }
  return restore
}
