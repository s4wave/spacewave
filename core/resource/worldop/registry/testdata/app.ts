import { z } from 'zod'
import { defineApp } from '../../../../../sdk/sync/app.js'
import { definePlugin } from '../../../../../sdk/sync/plugin.js'

/** step is replaced when compiling each immutable test executable. */
declare const STEP: number

const number = z.number().int().nonnegative()
const app = defineApp(
  {
    id: 'test/colors',
    version: 1,
    collections: { counts: number },
    mutations: {
      like: { input: z.string(), output: number },
      fail: { input: z.string(), output: number },
      cancel: { input: z.string(), output: number },
    },
  },
  {
    async like({ collection }, key) {
      const counts = collection('counts')
      const total = ((await counts.get(key)) ?? 0) + STEP
      await counts.put(key, total)
      return total
    },
    async cancel({ collection, signal }, key) {
      await collection('counts').put(key, 999)
      await new Promise<never>((_, reject) => {
        signal.addEventListener('abort', () => reject(signal.reason), {
          once: true,
        })
        if (signal.aborted) reject(signal.reason)
      })
      return 999
    },
    async fail({ collection }, key) {
      await collection('counts').put(key, 999)
      throw new Error('Rejected after staging a write')
    },
  },
)

export default definePlugin({
  app,
  displayName: 'Test colors',
  viewer: { entry: 'colors.tsx', componentId: 'test-colors' },
})
