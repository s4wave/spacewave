import { describe, expect, it, vi } from 'vitest'

import { findExistingSessionIndexByUsername } from './find-existing-session.js'

describe('findExistingSessionIndexByUsername', () => {
  it('keeps one metadata read in flight and selects the lowest match in list order', async () => {
    const calls: number[] = []
    const resolvers: Array<() => void> = []
    let inFlight = 0
    let maxInFlight = 0
    const root = {
      listSessions: vi.fn().mockResolvedValue({
        sessions: [
          { sessionIndex: 9 },
          { sessionIndex: 0 },
          { sessionIndex: 3 },
          { sessionIndex: 6 },
        ],
      }),
      getSessionMetadata: vi
        .fn()
        .mockImplementation(async (sessionIndex: number) => {
          calls.push(sessionIndex)
          inFlight++
          maxInFlight = Math.max(maxInFlight, inFlight)
          await new Promise<void>((resolve) => {
            resolvers.push(resolve)
          })
          inFlight--
          return {
            metadata: {
              cloudEntityId: sessionIndex === 9 ? 'other-user' : 'casey',
            },
          }
        }),
    }

    const lookup = findExistingSessionIndexByUsername(root, 'casey')
    await vi.waitFor(() => expect(calls).toEqual([9]))
    resolvers.shift()?.()
    await vi.waitFor(() => expect(calls).toEqual([9, 3]))
    resolvers.shift()?.()
    await vi.waitFor(() => expect(calls).toEqual([9, 3, 6]))
    resolvers.shift()?.()

    await expect(lookup).resolves.toBe(3)
    expect(maxInFlight).toBe(1)
  })

  it('returns null when no mounted session matches the username', async () => {
    const root = {
      listSessions: vi.fn().mockResolvedValue({
        sessions: [{ sessionIndex: 3 }],
      }),
      getSessionMetadata: vi.fn().mockResolvedValue({
        metadata: { cloudEntityId: 'other-user' },
      }),
    }

    await expect(
      findExistingSessionIndexByUsername(root, 'casey'),
    ).resolves.toBeNull()
  })

  it('stops after a metadata rejection and forwards the abort signal', async () => {
    const error = new Error('metadata unavailable')
    const abortController = new AbortController()
    const root = {
      listSessions: vi.fn().mockResolvedValue({
        sessions: [
          { sessionIndex: 4 },
          { sessionIndex: 2 },
          { sessionIndex: 7 },
        ],
      }),
      getSessionMetadata: vi
        .fn()
        .mockResolvedValueOnce({ metadata: { cloudEntityId: 'casey' } })
        .mockRejectedValueOnce(error),
    }

    await expect(
      findExistingSessionIndexByUsername(root, 'casey', abortController.signal),
    ).rejects.toBe(error)
    expect(root.listSessions).toHaveBeenCalledWith(abortController.signal)
    expect(root.getSessionMetadata).toHaveBeenNthCalledWith(
      1,
      4,
      abortController.signal,
    )
    expect(root.getSessionMetadata).toHaveBeenNthCalledWith(
      2,
      2,
      abortController.signal,
    )
    expect(root.getSessionMetadata).toHaveBeenCalledTimes(2)
  })
})
