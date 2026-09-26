import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import type { StateAtomAccessor } from './useBackendStateAtom.js'
import { useBackendStateAtomValue } from './useBackendStateAtom.js'

function buildAccessor(value: StateAtomAccessor['value']): StateAtomAccessor {
  return {
    value,
    loading: value === null,
    error: null,
    retry: vi.fn(),
  }
}

function SelectedNoteButton({ accessor }: { accessor: StateAtomAccessor }) {
  const state = useBackendStateAtomValue(accessor, 'notes/selectedNote', '')
  return (
    <button
      type="button"
      data-testid="selected-note"
      onClick={() => state.setValue('welcome.md')}
    >
      {state.value || 'empty'}
    </button>
  )
}

describe('useBackendStateAtomValue', () => {
  afterEach(() => {
    cleanup()
  })

  it('keeps a state update visible while the backend atom is still loading', () => {
    render(<SelectedNoteButton accessor={buildAccessor(null)} />)

    const button = screen.getByTestId('selected-note')
    expect(button.textContent).toBe('empty')

    fireEvent.click(button)

    expect(button.textContent).toBe('welcome.md')
  })

  it('flushes a pending state update when the backend atom becomes available', async () => {
    const setState = vi.fn(() => Promise.resolve())
    const stateAtom = {
      setState,
      watchState: vi.fn(() => null),
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const accessStateAtom = vi.fn(() => Promise.resolve(stateAtom as never))

    const { rerender } = render(
      <SelectedNoteButton accessor={buildAccessor(null)} />,
    )
    fireEvent.click(screen.getByTestId('selected-note'))

    rerender(<SelectedNoteButton accessor={buildAccessor(accessStateAtom)} />)

    await waitFor(() =>
      expect(setState).toHaveBeenCalledWith('{"json":"welcome.md"}'),
    )
  })

  it('accepts remote changes after the backend confirms a pending write', async () => {
    const responses: { stateJson: string }[] = []
    let wake: (() => void) | null = null
    const push = (stateJson: string) => {
      responses.push({ stateJson })
      wake?.()
    }
    async function* watchState(_req: unknown, signal: AbortSignal) {
      while (!signal.aborted) {
        const next = responses.shift()
        if (next) {
          yield next
          continue
        }
        await new Promise<void>((resolve) => {
          wake = resolve
        })
      }
    }
    const setState = vi.fn((stateJson: string) => {
      push(stateJson)
      return Promise.resolve()
    })
    const stateAtom = {
      setState,
      watchState,
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const accessStateAtom = vi.fn(() => Promise.resolve(stateAtom as never))
    push('{"json":""}')

    render(<SelectedNoteButton accessor={buildAccessor(accessStateAtom)} />)
    const button = screen.getByTestId('selected-note')
    await waitFor(() => expect(button.textContent).toBe('empty'))

    fireEvent.click(button)
    await waitFor(() => expect(setState).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(button.textContent).toBe('welcome.md'))

    push('{"json":"remote.md"}')
    await waitFor(() => expect(button.textContent).toBe('remote.md'))
    expect(setState).toHaveBeenCalledTimes(1)
  })
})
