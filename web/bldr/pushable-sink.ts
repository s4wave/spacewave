import { Pushable } from 'it-pushable'
import { Source, Sink } from 'it-stream-types'
import { castToError } from './error.js'

export function buildPushableSink<T>(target: Pushable<T>): Sink<T> {
  return async function pushableSink(source: Source<T>): Promise<void> {
    try {
      for await (const pkt of source) {
        if (Array.isArray(pkt)) {
          for (const p of pkt) {
            target.push(p)
          }
        } else {
          target.push(pkt)
        }
      }
      target.end()
    } catch (err) {
      const error = castToError(err)
      target.end(error)
    }
  }
}
