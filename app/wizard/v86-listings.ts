import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { KeyedSet } from '@s4wave/sdk/world/entity-set.js'

// V86ListingsFailure is a failed read of the Space's images or VMs before
// the listing was complete: the title identifies the failed listing, and retry
// reads it again.
export interface V86ListingsFailure {
  title: string
  message: string
  retry: () => void
}

// V86ListingsStatus is what the image step may do with the listings.
export interface V86ListingsStatus {
  // pending is set until both listings are complete, so nothing seeds a
  // default source from them meanwhile.
  pending: boolean
  // failures lists each listing that could not be read, images first.
  // pending stays set while any is listed.
  failures: V86ListingsFailure[]
  // waiting identifies the listing still being read while none failed: the
  // images until their first read is complete, then the VMs.
  waiting: 'images' | 'vms' | null
  // stopped is set when the watch of a complete image set reached its
  // reconnect cap: the set stays and seeds, but may be out of date.
  stopped: V86ListingsFailure | null
}

/**
 * v86ListingsStatus derives the image step's state from the Space's image
 * set and VM listing, as V86Listings.tla specifies. The image set is pending
 * until its first read is complete: a watch yields after each page of that
 * read, and a default source seeded from a partial set could miss the newest
 * image.
 * While the watch reconnects by itself the resource loads again, so the step
 * keeps loading before the set is complete and stays quiet after. The
 * resource fails only once the watch stops at its reconnect cap. Before the
 * set is complete, a stopped watch is a failure, whether or not the watch had
 * yielded a page. After completion the complete set stays, but may be out of
 * date, so the set is stopped.
 */
export function v86ListingsStatus(
  images: Resource<KeyedSet<unknown>>,
  vms: Resource<unknown>,
): V86ListingsStatus {
  const imageSet = resourceContent(images)
  const failures: V86ListingsFailure[] = []
  if (!imageSet?.complete && images.error) {
    failures.push(
      failureOf('Could not read the VM images in this Space', images),
    )
  }
  if (vms.error) {
    failures.push(failureOf('Could not read the VMs in this Space', vms))
  }
  let waiting: V86ListingsStatus['waiting'] = null
  if (failures.length === 0) {
    if (!imageSet?.complete) waiting = 'images'
    else if (vms.loading) waiting = 'vms'
  }
  const stops = imageSet?.complete && images.error
  return {
    pending: !imageSet?.complete || vms.loading || !!vms.error,
    failures,
    waiting,
    stopped: stops ? failureOf('This list may be out of date', images) : null,
  }
}

// failureOf is the failure of the listing resource read, under title.
function failureOf(
  title: string,
  resource: Resource<unknown>,
): V86ListingsFailure {
  return {
    title,
    message: resource.error?.message ?? '',
    retry: resource.retry,
  }
}
