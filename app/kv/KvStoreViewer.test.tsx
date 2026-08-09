import { useEffect, useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { KvKeyEntry } from '@s4wave/sdk/kv/kv.js'

// FakeKvStore is an in-memory KvStore stand-in driving the viewer through the
// same handle API the real SDK handle exposes.
class FakeKvStore {
  public readonly id = 1
  private readonly store = new Map<string, Uint8Array>()
  public failNextWrite = false
  public setCalls = 0
  public deleteCalls = 0

  constructor(seed: Record<string, string>) {
    const encoder = new TextEncoder()
    for (const [key, value] of Object.entries(seed)) {
      this.store.set(key, encoder.encode(value))
    }
  }

  scanKeys(): Promise<KvKeyEntry[]> {
    const encoder = new TextEncoder()
    return Promise.resolve(
      [...this.store.entries()].map(([key, value]) => ({
        key: encoder.encode(key),
        byteLength: value.length,
      })),
    )
  }

  get(key: Uint8Array): Promise<{ data: Uint8Array; found: boolean }> {
    const label = new TextDecoder().decode(key)
    const data = this.store.get(label)
    return Promise.resolve({
      data: data ?? new Uint8Array(),
      found: data != null,
    })
  }

  async withTransaction<T>(
    _write: boolean,
    fn: (tx: FakeTx) => Promise<T>,
  ): Promise<T> {
    if (this.failNextWrite) {
      this.failNextWrite = false
      throw new Error('transaction failed')
    }
    return fn(new FakeTx(this))
  }

  setKey(key: Uint8Array, value: Uint8Array) {
    this.setCalls++
    this.store.set(new TextDecoder().decode(key), value)
  }

  deleteKey(key: Uint8Array) {
    this.deleteCalls++
    this.store.delete(new TextDecoder().decode(key))
  }
}

class FakeTx {
  constructor(private readonly owner: FakeKvStore) {}
  set(key: Uint8Array, value: Uint8Array): Promise<void> {
    this.owner.setKey(key, value)
    return Promise.resolve()
  }
  delete(key: Uint8Array): Promise<void> {
    this.owner.deleteKey(key)
    return Promise.resolve()
  }
}

let fakeStore: FakeKvStore

const handleResource = () => ({
  value: fakeStore,
  loading: false,
  error: null,
  retry: vi.fn(),
})

// A useResource mock that actually runs the async factory against the fake
// handle and re-runs it when retry() is called, mirroring the real hook.
function useResourceMock<P, T>(
  parent: { value: P | null },
  factory: (parent: P, signal: AbortSignal) => Promise<T>,
  _deps: unknown[],
) {
  const [value, setValue] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [generation, setGeneration] = useState(0)
  const parentValue = parent.value

  useEffect(() => {
    let cancelled = false
    if (parentValue == null) return
    setLoading(true)
    setError(null)
    factory(parentValue, new AbortController().signal).then(
      (result) => {
        if (cancelled) return
        setValue(result)
        setLoading(false)
      },
      (err: unknown) => {
        if (cancelled) return
        setError(err instanceof Error ? err : new Error(String(err)))
        setLoading(false)
      },
    )
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [parentValue, generation])

  return { value, loading, error, retry: () => setGeneration((g) => g + 1) }
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => handleResource(),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (parent: never, factory: never, deps: never) =>
    useResourceMock(parent, factory, deps),
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: () => 'kv/test-store',
}))

import { KvStoreViewer } from './KvStoreViewer.js'

function renderViewer() {
  return render(<KvStoreViewer objectInfo={{}} worldState={{} as never} />)
}

describe('KvStoreViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the watched key list with byte lengths', async () => {
    fakeStore = new FakeKvStore({ alpha: 'hi', beta: 'longer-value' })
    renderViewer()

    await waitFor(() => expect(screen.getByText('alpha')).toBeTruthy())
    expect(screen.getByText('beta')).toBeTruthy()
    expect(screen.getByText('2 B')).toBeTruthy()
    expect(screen.getByText('12 B')).toBeTruthy()
    expect(screen.getByText('2 keys')).toBeTruthy()
  })

  it('filters by prefix and toggles sort direction', async () => {
    fakeStore = new FakeKvStore({ apple: 'a', apricot: 'b', banana: 'c' })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('apple')).toBeTruthy())

    await user.type(screen.getByLabelText('Filter by key prefix'), 'ap')
    expect(screen.queryByText('banana')).toBeNull()
    expect(screen.getByText('apple')).toBeTruthy()
    expect(screen.getByText('apricot')).toBeTruthy()

    const options = () =>
      screen.getAllByRole('option').map((el) => el.textContent)
    expect(options()[0]).toContain('apple')
    await user.click(screen.getByLabelText('Sort keys descending'))
    expect(options()[0]).toContain('apricot')
  })

  it('shows the selected value with the detail pane and auto-detected mode', async () => {
    fakeStore = new FakeKvStore({ doc: '{"a":1}' })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('doc')).toBeTruthy())
    await user.click(screen.getByRole('option'))

    const editor = await screen.findByLabelText('Key value')
    await waitFor(() =>
      expect((editor as HTMLTextAreaElement).value).toContain('"a": 1'),
    )
    expect(
      screen.getByRole('radio', { name: 'JSON' }).getAttribute('aria-checked'),
    ).toBe('true')
  })

  it('creates a key through a write transaction', async () => {
    fakeStore = new FakeKvStore({})
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() =>
      expect(screen.getByText('Select a key to view its value.')).toBeTruthy(),
    )
    await user.click(screen.getByRole('button', { name: 'New Key' }))
    await user.type(screen.getByLabelText('New key'), 'fresh')
    await user.type(screen.getByLabelText('New key value'), 'value')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(fakeStore.setCalls).toBe(1))
    await waitFor(() =>
      expect(
        screen
          .getAllByText('fresh')
          .some((el) => el.closest('[role="option"]')),
      ).toBe(true),
    )
  })

  it('updates a key with dirty tracking and save', async () => {
    fakeStore = new FakeKvStore({ name: 'old' })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('name')).toBeTruthy())
    await user.click(screen.getByRole('option'))

    const editor = await screen.findByLabelText('Key value')
    expect(screen.getByRole('button', { name: 'Save' })).toHaveProperty(
      'disabled',
      true,
    )
    await user.clear(editor)
    await user.type(editor, 'new')
    expect(screen.getByRole('button', { name: 'Save' })).toHaveProperty(
      'disabled',
      false,
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(fakeStore.setCalls).toBe(1))
  })

  it('deletes a key behind a confirm affordance', async () => {
    fakeStore = new FakeKvStore({ gone: 'x' })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('gone')).toBeTruthy())
    await user.click(screen.getByRole('option'))
    await screen.findByLabelText('Key value')

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(screen.getByText('Delete key?')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))

    await waitFor(() => expect(fakeStore.deleteCalls).toBe(1))
  })

  it('surfaces an inline error when a write transaction fails', async () => {
    fakeStore = new FakeKvStore({ name: 'old' })
    fakeStore.failNextWrite = true
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('name')).toBeTruthy())
    await user.click(screen.getByRole('option'))
    const editor = await screen.findByLabelText('Key value')
    await user.clear(editor)
    await user.type(editor, 'new')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(screen.getByText('Save failed: transaction failed')).toBeTruthy(),
    )
    expect(fakeStore.setCalls).toBe(0)
  })

  it('shows a retryable error state when the key list fails to load', async () => {
    fakeStore = new FakeKvStore({})
    vi.spyOn(fakeStore, 'scanKeys').mockRejectedValue(new Error('boom'))
    renderViewer()

    await waitFor(() =>
      expect(screen.getByText('KV store unavailable')).toBeTruthy(),
    )
    expect(screen.getByText('Error: boom')).toBeTruthy()
  })
})
