import { describe, it, expect } from 'vitest'
import {
  sortSessionsNewestFirst,
  type SessionWithMeta,
} from './useSessionList.js'

describe('sortSessionsNewestFirst', () => {
  it('sorts by created_at descending', () => {
    const sessions: SessionWithMeta[] = [
      { entry: { sessionIndex: 1 }, metadata: { createdAt: 1000n } },
      { entry: { sessionIndex: 2 }, metadata: { createdAt: 3000n } },
      { entry: { sessionIndex: 3 }, metadata: { createdAt: 2000n } },
    ]
    const sorted = sortSessionsNewestFirst(sessions)
    expect(sorted.map((s) => s.entry.sessionIndex)).toEqual([2, 3, 1])
  })

  it('falls back to session_index descending when created_at is zero', () => {
    const sessions: SessionWithMeta[] = [
      { entry: { sessionIndex: 1 }, metadata: { createdAt: 0n } },
      { entry: { sessionIndex: 3 }, metadata: { createdAt: 0n } },
      { entry: { sessionIndex: 2 } },
    ]
    const sorted = sortSessionsNewestFirst(sessions)
    expect(sorted.map((s) => s.entry.sessionIndex)).toEqual([3, 2, 1])
  })

  it('sorts non-zero before zero created_at', () => {
    const sessions: SessionWithMeta[] = [
      { entry: { sessionIndex: 5 }, metadata: { createdAt: 0n } },
      { entry: { sessionIndex: 1 }, metadata: { createdAt: 1000n } },
    ]
    const sorted = sortSessionsNewestFirst(sessions)
    expect(sorted.map((s) => s.entry.sessionIndex)).toEqual([1, 5])
  })

  it('does not mutate the input array', () => {
    const sessions: SessionWithMeta[] = [
      { entry: { sessionIndex: 2 }, metadata: { createdAt: 1000n } },
      { entry: { sessionIndex: 1 }, metadata: { createdAt: 2000n } },
    ]
    sortSessionsNewestFirst(sessions)
    expect(sessions[0].entry.sessionIndex).toBe(2)
  })
})
