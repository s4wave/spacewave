import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'

import { Counter, CounterTypeID } from './counter.js'

// CounterViewer renders the value stored by the custom TypeScript ObjectType fixture.
export function CounterViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const counter = useAccessTypedHandle(
    worldState,
    objectKey,
    Counter,
    CounterTypeID,
  )
  const value = useResource(
    counter,
    async (handle, signal) => {
      if (!handle) return null
      return handle.getCounter(signal)
    },
    [],
  )

  if (value.error) return <p>Counter unavailable</p>
  if (value.value == null) return <p>Loading counter</p>
  return <output aria-label="Counter value">{value.value.toString()}</output>
}
