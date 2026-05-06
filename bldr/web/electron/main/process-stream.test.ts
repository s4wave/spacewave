import { describe, expect, it, vi } from 'vitest'

import {
  ignoreClosedProcessStreamErrors,
  isClosedProcessStreamError,
  type ProcessStream,
} from './process-stream.js'

describe('electron main process stream handling', () => {
  it('recognizes closed process stream errors', () => {
    const err = new Error('write EPIPE') as Error & { code: string }
    err.code = 'EPIPE'
    expect(isClosedProcessStreamError(err)).toBe(true)
  })

  it('installs stdout and stderr error handlers', () => {
    const stdout = { on: vi.fn() } satisfies ProcessStream
    const stderr = { on: vi.fn() } satisfies ProcessStream

    ignoreClosedProcessStreamErrors({ stdout, stderr })

    expect(stdout.on).toHaveBeenCalledWith('error', expect.any(Function))
    expect(stderr.on).toHaveBeenCalledWith('error', expect.any(Function))
  })
})
