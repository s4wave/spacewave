import type { RemoteResourceClient } from './tracked-client.js'

/** ResourceOffers owns one invocation's unaccepted children until transfer or cleanup. */
export class ResourceOffers {
  /** ended prevents a completed invocation from registering another child. */
  ended = false
  private unpublished: Set<number> | undefined
  private published: Set<number> | undefined
  private sending: ReturnType<typeof Promise.withResolvers<void>> | undefined

  constructor(private readonly client: RemoteResourceClient) {}

  /** register retains an unpublished child before its response is sent. */
  register(id: number): void {
    ;(this.unpublished ??= new Set()).add(id)
  }

  /** accept consumes an offer on adoption or explicit release. */
  accept(id: number): void {
    this.unpublished?.delete(id)
    this.published?.delete(id)
  }

  /** publish retains in-flight offers until the response sink accepts or rejects them. */
  async *publish(
    source: AsyncIterable<Uint8Array> | Iterable<Uint8Array>,
  ): AsyncIterable<Uint8Array> {
    for await (const message of source) {
      if (this.ended) return
      if (!this.unpublished?.size) {
        yield message
        continue
      }
      const ids = [...this.unpublished]
      const sending = Promise.withResolvers<void>()
      this.sending = sending
      let accepted = false
      try {
        yield message
        accepted = true
      } finally {
        // A successful sink asks for the next message; a failed sink returns
        // the iterator instead. Adoption may already have consumed an ID.
        if (accepted) {
          for (const id of ids) {
            if (this.unpublished.delete(id))
              (this.published ??= new Set()).add(id)
          }
        }
        this.sending = undefined
        sending.resolve()
      }
    }
  }

  /** finish seals registration without delaying propagation of a handler failure. */
  finish(): void {
    this.ended = true
    if (this.sending) {
      void this.sending.promise.then(() => this.release(this.unpublished))
    } else {
      this.release(this.unpublished)
    }
  }

  /** abandon reclaims only the offers that adoption has not consumed. */
  abandon(): void {
    this.ended = true
    this.release(this.unpublished)
    this.release(this.published)
  }

  /** release drains the selected offers even when a native cleanup callback throws. */
  private release(ids: Set<number> | undefined): void {
    if (!ids) return
    for (const id of ids) {
      try {
        this.client.releaseResource(id)
      } catch (error) {
        console.error('Error releasing resource offer:', error)
      }
    }
  }
}
