import { useSyncExternalStore } from 'react'

import {
  readBootDownloads,
  subscribeBootDownloads,
  type BootDownload,
} from '@aptre/bldr'

export type { BootDownload }

// useBootDownloads subscribes the loading screen to the boot download registry
// owned by the bldr fetch substrate. The registry replaces its array reference
// on every change, so the snapshot doubles as the change key with no polling.
export function useBootDownloads(): BootDownload[] {
  return useSyncExternalStore(
    subscribeBootDownloads,
    readBootDownloads,
    readBootDownloads,
  )
}
