import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ItState } from './it-state.js'

describe('ItState', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('should create an instance with default values', () => {
    const state = new ItState()
    expect(state).toBeDefined()
  })

  it('should return undefined snapshot when no getSnapshot function is provided', async () => {
    const state = new ItState()
    const snapshot = await state.snapshot
    expect(snapshot).toBeUndefined()
  })

  it('should return snapshot from provided getSnapshot function', async () => {
    const mockSnapshot = { data: 'test' }
    const state = new ItState(async () => mockSnapshot)
    const snapshot = await state.snapshot
    expect(snapshot).toEqual(mockSnapshot)
  })

  it('should emit initial snapshot in iterable', async () => {
    const mockSnapshot = { data: 'initial' }
    const state = new ItState(async () => mockSnapshot)

    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    const result = await iterator.next()
    expect(result.done).toBe(false)
    expect(result.value).toEqual(mockSnapshot)

    // Clean up
    await iterator.return!()
  })

  it('should emit pushed events to subscribers', async () => {
    const state = new ItState<string>()
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Push an event after a small delay
    setTimeout(() => {
      state.pushChangeEvent('event1')
    }, 100)

    vi.advanceTimersByTime(100)
    const result = await iterator.next()
    expect(result.done).toBe(false)
    expect(result.value).toBe('event1')

    // Clean up
    await iterator.return!()
  })

  it('should handle multiple events in sequence', async () => {
    const state = new ItState<number>()
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Push events in sequence
    setTimeout(() => {
      state.pushChangeEvent(1)
      state.pushChangeEvent(2)
      state.pushChangeEvent(3)
    }, 100)

    vi.advanceTimersByTime(100)

    const result1 = await iterator.next()
    expect(result1.value).toBe(1)

    const result2 = await iterator.next()
    expect(result2.value).toBe(2)

    const result3 = await iterator.next()
    expect(result3.value).toBe(3)

    // Clean up
    await iterator.return!()
  })

  it('should handle mostRecentOnly option', async () => {
    const state = new ItState<number>(undefined, { mostRecentOnly: true })
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Push multiple events rapidly
    setTimeout(() => {
      state.pushChangeEvent(1)
      state.pushChangeEvent(2)
      state.pushChangeEvent(3)
    }, 100)

    vi.advanceTimersByTime(100)

    // Should only get the most recent value (3)
    const result = await iterator.next()
    expect(result.value).toBe(3)

    // Clean up
    await iterator.return!()
  })

  it('should handle errors in getSnapshot', async () => {
    const error = new Error('Snapshot error')
    const state = new ItState(async () => {
      throw error
    })

    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    try {
      await iterator.next()
      // Should not reach here
      expect(true).toBe(false)
    } catch (e) {
      expect(e).toBe(error)
    }

    // Clean up
    await iterator.return!()
  })

  it('should handle pushSnapshot correctly', async () => {
    const mockSnapshot = { data: 'snapshot' }
    const state = new ItState(async () => mockSnapshot)
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Get initial snapshot
    const initialResult = await iterator.next()
    expect(initialResult.value).toEqual(mockSnapshot)

    // Push updated snapshot
    setTimeout(async () => {
      await state.pushSnapshot()
    }, 100)

    vi.advanceTimersByTime(100)

    // Should get the snapshot again
    const updatedResult = await iterator.next()
    expect(updatedResult.value).toEqual(mockSnapshot)

    // Clean up
    await iterator.return!()
  })

  it('should handle errors in pushSnapshot', async () => {
    const error = new Error('Snapshot error')
    const getSnapshotMock = vi.fn()
    getSnapshotMock.mockImplementationOnce(async () => ({ data: 'initial' }))
    getSnapshotMock.mockImplementationOnce(async () => {
      throw error
    })

    const state = new ItState(getSnapshotMock)
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Get initial snapshot
    const initialResult = await iterator.next()
    expect(initialResult.value).toEqual({ data: 'initial' })

    // Mock error handler to verify it's called
    const errorHandler = vi.fn()
    state['errorListeners'].add(errorHandler)

    // Push snapshot that will error
    await state.pushSnapshot()

    // Verify error handler was called
    expect(errorHandler).toHaveBeenCalledWith(error)

    // Clean up
    await iterator.return!()
  })

  it('should clean up listeners when iterator is done', async () => {
    const state = new ItState<string>()
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Verify listener was added
    expect(state['listeners'].size).toBe(1)
    expect(state['errorListeners'].size).toBe(1)

    // Complete the iterator
    await iterator.return!()

    // Verify listeners were removed
    expect(state['listeners'].size).toBe(0)
    expect(state['errorListeners'].size).toBe(0)
  })

  it('should handle multiple iterators independently', async () => {
    const state = new ItState<string>()

    const iterator1 = state.getIterable()[Symbol.asyncIterator]()
    const iterator2 = state.getIterable()[Symbol.asyncIterator]()

    // Push an event
    setTimeout(() => {
      state.pushChangeEvent('event')
    }, 100)

    vi.advanceTimersByTime(100)

    // Both iterators should receive the event
    const result1 = await iterator1.next()
    expect(result1.value).toBe('event')

    const result2 = await iterator2.next()
    expect(result2.value).toBe('event')

    // Clean up
    await iterator1.return!()
    await iterator2.return!()
  })

  it('should drop intermediate values with mostRecentOnly option', async () => {
    const state = new ItState<number>(undefined, { mostRecentOnly: true })
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Push multiple events without awaiting next()
    state.pushChangeEvent(1)
    state.pushChangeEvent(2)
    state.pushChangeEvent(3)

    // Should only get the most recent value (3)
    const result = await iterator.next()
    expect(result.value).toBe(3)

    // Push more events
    state.pushChangeEvent(4)
    state.pushChangeEvent(5)

    // Should get the most recent value (5)
    const result2 = await iterator.next()
    expect(result2.value).toBe(5)

    // Clean up
    await iterator.return!()
  })

  it('should handle concurrent pushChangeEvent calls', async () => {
    const state = new ItState<number>()
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Start waiting for next value
    const nextPromise = iterator.next()

    // Push events while waiting
    state.pushChangeEvent(1)
    state.pushChangeEvent(2)

    // Should get the first value
    const result = await nextPromise
    expect(result.value).toBe(1)

    // Second value should be queued
    const result2 = await iterator.next()
    expect(result2.value).toBe(2)

    // Clean up
    await iterator.return!()
  })

  it('should handle multiple iterators with mostRecentOnly independently', async () => {
    const state = new ItState<number>(undefined, { mostRecentOnly: true })

    const iterator1 = state.getIterable()[Symbol.asyncIterator]()
    const iterator2 = state.getIterable()[Symbol.asyncIterator]()

    // Push initial events
    state.pushChangeEvent(1)

    // First iterator gets the value
    const result1 = await iterator1.next()
    expect(result1.value).toBe(1)

    // Push more events
    state.pushChangeEvent(2)
    state.pushChangeEvent(3)

    // Second iterator should get the most recent value
    const result2 = await iterator2.next()
    expect(result2.value).toBe(3)

    // First iterator should also get the most recent value
    const result1b = await iterator1.next()
    expect(result1b.value).toBe(3)

    // Clean up
    await iterator1.return!()
    await iterator2.return!()
  })

  it('should handle getSnapshot returning undefined', async () => {
    // Create a state with a getSnapshot that returns undefined
    const state = new ItState<string>(async () => undefined)
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Push an event after a delay
    setTimeout(() => {
      state.pushChangeEvent('event')
    }, 100)

    vi.advanceTimersByTime(100)

    // First value should be the pushed event, not a snapshot
    const result = await iterator.next()
    expect(result.value).toBe('event')

    // Clean up
    await iterator.return!()
  })

  it('should handle cancellation while waiting for a value', async () => {
    const state = new ItState<string>()
    const iterable = state.getIterable()
    const iterator = iterable[Symbol.asyncIterator]()

    // Start waiting for a value
    const nextPromise = iterator.next()

    // Cancel the iterator (this should resolve the pending next() call)
    await iterator.return!()

    // Push an event after cancellation
    state.pushChangeEvent('event')

    // Verify listeners were removed
    expect(state['listeners'].size).toBe(0)
    expect(state['errorListeners'].size).toBe(0)

    // The promise should resolve with done: true
    const result = await nextPromise
    expect(result.done).toBe(true)
  }, 10000) // Increase timeout to avoid test timeout
})
