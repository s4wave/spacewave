// asyncValues exposes finite test fixtures through the same asynchronous
// iteration boundary as streaming production APIs.
export function asyncValues<T>(...values: T[]): AsyncIterable<T> {
  return {
    [Symbol.asyncIterator](): AsyncIterator<T> {
      const iterator = values.values()
      return {
        next: () => Promise.resolve(iterator.next()),
      }
    },
  }
}
