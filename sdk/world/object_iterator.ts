import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  ObjectIteratorResourceService,
  ObjectIteratorResourceServiceClient,
} from './world_srpc.pb.js'

// ObjectIterator iterates over objects in a WorldState.
// Always call close() when done with the iterator.
// ObjectIterator functions are NOT thread safe, use it from one goroutine at a time.
export class ObjectIterator extends Resource {
  private service: ObjectIteratorResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new ObjectIteratorResourceServiceClient(resourceRef.client)
  }

  // Err returns any error that has closed the iterator.
  // May return context.Canceled if closed.
  public async err(abortSignal?: AbortSignal): Promise<string | null> {
    const response = await this.service.Err({}, abortSignal)
    return response.error || null
  }

  // Valid returns if the iterator points to a valid entry.
  public async valid(abortSignal?: AbortSignal): Promise<boolean> {
    const response = await this.service.Valid({}, abortSignal)
    return response.valid ?? false
  }

  // Key returns the current entry key, or empty string if not valid.
  public async key(abortSignal?: AbortSignal): Promise<string> {
    const response = await this.service.Key({}, abortSignal)
    return response.objectKey ?? ''
  }

  // Next advances to the next entry and returns Valid.
  public async next(abortSignal?: AbortSignal): Promise<boolean> {
    const response = await this.service.Next({}, abortSignal)
    return response.valid ?? false
  }

  // Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
  // Pass empty string to seek to the beginning (or end if reversed).
  public async seek(k: string, abortSignal?: AbortSignal): Promise<void> {
    await this.service.Seek({ objectKey: k }, abortSignal)
  }

  // Close releases the iterator.
  public async close(abortSignal?: AbortSignal): Promise<void> {
    await this.service.Close({}, abortSignal)
  }
}
