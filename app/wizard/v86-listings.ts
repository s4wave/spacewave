import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { KeyedSet } from '@s4wave/sdk/world/entity-set.js'

// V86ListingsFailure is a failed read of the Space's images or VMs before
// the listing was complete: the title names which listing failed, and retry
// reads it again.
export interface V86ListingsFailure {
  title: string
  message: string
  retry: () => void
}

// V86ListingsStatus is what the image step may do with the listings.
export interface V86ListingsStatus {
  // pending is set until both listings are complete; nothing seeds a
  // default source from them meanwhile.
  pending: boolean
  // failures lists each listing that could not be read, images first;
  // pending stays set while any is listed.
  failures: V86ListingsFailure[]
}

/**
 * v86ListingsStatus derives the image step's state from the Space's image
 * set and VM listing, as V86Listings.tla specifies. The image set is pending
 * until its first read is complete: a watch yields after each page of that
 * read, and a default seeded from a partial set could miss the newest image.
 * An image read that fails before the set is complete is a failure, whether
 * or not it had yielded a page; one that fails after keeps the complete set
 * and reopens by itself, so it stays quiet.
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
  return {
    pending: !imageSet?.complete || vms.loading || !!vms.error,
    failures,
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
