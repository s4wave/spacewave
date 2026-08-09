import { useSyncExternalStore } from 'react'

import { getAppPath, subscribeAppPath } from './app-path.js'

export function useAppPath(): string {
  return useSyncExternalStore(subscribeAppPath, getAppPath, getAppPath)
}
