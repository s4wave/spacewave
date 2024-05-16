/**
 * This file (only) is dual licensed under MIT and Apache-2.0.
 * MIT: https://www.opensource.org/licenses/mit
 * Apache-2.0: https://www.apache.org/licenses/license-2.0
 * Upstream: https://raw.githubusercontent.com/achingbrain/it/main/packages/browser-readablestream-to-it/src/index.ts
*/

/**
 * @packageDocumentation
 *
 * Allows treating a browser readable stream as an async iterator.
 *
 * @example
 *
 * ```javascript
 * import toIt from 'browser-readablestream-to-it'
 * import all from 'it-all'
 *
 * const content = [0, 1, 2, 3, 4]
 *
 * const stream = new ReadableStream({
 *   start(controller) {
 *     for (let i = 0; i < content.length; i++) {
 *       controller.enqueue(content[i])
 *     }
 *
 *     controller.close()
 *   }
 * })
 *
 * const arr = await all(toIt(stream))
 *
 * console.info(arr) // 0, 1, 2, 3, 4
 * ```
 *
 * ## preventCancel
 *
 * By default a readable stream will have [.cancel](https://developer.mozilla.org/en-US/docs/Web/API/ReadableStream/cancel) called on it once it has ended or
 * reading has stopped prematurely.
 *
 * To prevent this behaviour, pass `preventCancel: true` as an option:
 *
 * ```javascript
 * const arr = await all(toIt(stream, { preventCancel: true }))
 *
 * console.info(arr) // 0, 1, 2, 3, 4
 * ```
 */

export interface BrowserReadableStreamToItOptions {
  preventCancel?: boolean
}

/**
 * Turns a browser readable stream into an async iterable. Async iteration over
 * returned iterable will lock give stream, preventing any other consumer from
 * acquiring a reader. The lock will be released if iteration loop is broken. To
 * prevent stream cancelling optional `{ preventCancel: true }` could be passed
 * as a second argument.
 */
export default async function * browserReadableStreamToIt <T> (stream: ReadableStream<T>, options: BrowserReadableStreamToItOptions = {}): AsyncGenerator<T, void, undefined> {
  const reader = stream.getReader()

  try {
    while (true) {
      const result = await reader.read()

      if (result.done) {
        return
      }

      yield result.value
    }
  } finally {
    if (options.preventCancel !== true) {
      await reader.cancel()
    }

    reader.releaseLock()
  }
}
