import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { RuntimeClientClosedError } from '@aptre/bldr'
import { ResourceClientError } from '@aptre/bldr-sdk/resource/client.js'

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

  it('persists only the winner of same-tick updates', async () => {
    const setState = vi.fn(() => Promise.resolve())
    const stateAtom = {
      setState,
      watchState: vi.fn(() => null),
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const accessor = buildAccessor(
      vi.fn(() => Promise.resolve(stateAtom as never)),
    )
    function Menu() {
      const menu = useBackendStateAtomValue(accessor, 'openMenu', 'plugins')
      return (
        <button
          type="button"
          onClick={() => {
            menu.setValue('system-status')
            menu.setValue((current) => (current === 'plugins' ? '' : current))
          }}
        >
          {menu.value || 'closed'}
        </button>
      )
    }
    render(<Menu />)
    fireEvent.click(screen.getByRole('button', { name: 'plugins' }))
    expect(screen.getByRole('button').textContent).toBe('system-status')
    await waitFor(() =>
      expect(setState).toHaveBeenCalledWith('{"json":"system-status"}'),
    )
    expect(setState).toHaveBeenCalledTimes(1)
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

  describe.each([
    {
      name: 'ends quietly when the runtime client closes during a write',
      err: new RuntimeClientClosedError('normal-close', 'client', 1),
      logged: false,
    },
    {
      name: 'ends quietly when the Resource client connection drops during a write',
      err: new ResourceClientError(
        'ResourceClient control stream closed',
        'CONNECTION_FAILED',
      ),
      logged: false,
    },
    {
      name: 'logs a write that fails for another reason',
      err: new Error('write failed'),
      logged: true,
    },
  ])('write failure', ({ name, err, logged }) => {
    it(name, async () => {
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      const setState = vi.fn(() => Promise.reject(err))
      const stateAtom = {
        setState,
        watchState: vi.fn(() => null),
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }
      const accessStateAtom = vi.fn(() => Promise.resolve(stateAtom as never))

      render(<SelectedNoteButton accessor={buildAccessor(accessStateAtom)} />)
      fireEvent.click(screen.getByTestId('selected-note'))

      await waitFor(() => expect(setState).toHaveBeenCalledTimes(1))
      await Promise.resolve()
      if (logged) {
        await waitFor(() => expect(consoleError).toHaveBeenCalled())
      } else {
        expect(consoleError).not.toHaveBeenCalled()
      }
      consoleError.mockRestore()
    })
  })
})
