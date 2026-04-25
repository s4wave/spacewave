import { describe, expect, it, vi } from 'vitest'

import { findExistingSessionIndexByUsername } from './find-existing-session.js'

describe('findExistingSessionIndexByUsername', () => {
  it('returns the earliest matching session index for the requested username', async () => {
    const root = {
      listSessions: vi.fn().mockResolvedValue({
        sessions: [
          { sessionIndex: 5 },
          { sessionIndex: 2 },
          { sessionIndex: 7 },
        ],
      }),
      getSessionMetadata: vi
        .fn()
        .mockImplementation(async (sessionIndex: number) => {
          if (sessionIndex === 5) {
            return { metadata: { cloudEntityId: 'other-user' } }
          }
          if (sessionIndex === 2) {
            return { metadata: { cloudEntityId: 'casey' } }
          }
          if (sessionIndex === 7) {
            return { metadata: { cloudEntityId: 'casey' } }
          }
          return { metadata: null }
        }),
    }

    await expect(
      findExistingSessionIndexByUsername(root, 'casey'),
    ).resolves.toBe(2)
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
})
