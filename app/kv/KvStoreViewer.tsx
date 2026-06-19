import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LuDatabase } from 'react-icons/lu'

import { KvStore, KvStoreTypeID } from '@s4wave/sdk/kv/kv.js'

export { KvStoreTypeID }

// KvStoreViewer displays the first kv/store status surface.
export function KvStoreViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    KvStore,
    KvStoreTypeID,
  )
  const countResource = useResource(
    handle,
    async (kv, signal) => {
      if (!kv) return null
      return kv.keyCount(signal)
    },
    [],
  )

  const keyCount = countResource.value

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuDatabase className="text-foreground-alt size-3.5" aria-hidden />
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Key/Value Store
        </span>
      </div>
      <div className="flex-1 p-4">
        {countResource.loading && keyCount == null ? (
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading key count',
              detail: 'Reading the KV store root.',
            }}
          />
        ) : null}
        {countResource.error ? (
          <LoadingCard
            view={{
              state: 'error',
              title: 'KV store unavailable',
              detail: 'The typed handle could not read this object.',
              error: String(countResource.error),
              onRetry: countResource.retry,
            }}
          />
        ) : null}
        {keyCount != null ? (
          <div className="space-y-3">
            <div className="border-foreground/8 max-w-xs rounded-lg border p-3">
              <div className="text-foreground text-2xl font-semibold tabular-nums">
                {keyCount.toString()}
              </div>
              <div className="text-foreground-alt mt-1 text-xs">keys</div>
            </div>
            <p className="text-foreground-alt max-w-lg text-sm">
              Bytes-only KVTX store backed by this Space world object.
            </p>
          </div>
        ) : null}
      </div>
    </div>
  )
}
