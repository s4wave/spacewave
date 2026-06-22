// ReadableStream is the QuickJS plugin-host polyfill for the WHATWG default
// readable stream. It implements the default-reader subset used by the plugin
// SDK; BYOB readers, tee, and pipe are not provided.
export declare class ReadableStream<R = Uint8Array> {
  constructor(
    underlyingSource?: UnderlyingDefaultSource<R>,
    strategy?: QueuingStrategy<R>,
  )
  readonly locked: boolean
  getReader(): ReadableStreamDefaultReader<R>
  cancel(reason?: unknown): Promise<void>
}
