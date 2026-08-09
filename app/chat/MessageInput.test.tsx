import { Window } from 'happy-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'

import { MessageInput } from './MessageInput.js'

Object.defineProperties(globalThis, {
  document: { value: new Window().document, configurable: true },
  window: { value: new Window(), configurable: true },
})

afterEach(cleanup)

describe('MessageInput', () => {
  it('preserves a failed draft and retries it', async () => {
    const onSend = vi
      .fn<(text: string) => Promise<void>>()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce()
    const view = render(<MessageInput onSend={onSend} />)
    const input = view.getByPlaceholderText('Type a message…')
    fireEvent.change(input, { target: { value: 'hello' } })
    fireEvent.click(view.getByRole('button', { name: 'Send' }))
    await view.findByRole('alert')
    expect((input as HTMLTextAreaElement).value).toBe('hello')
    fireEvent.click(view.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect((input as HTMLTextAreaElement).value).toBe(''))
    expect(onSend).toHaveBeenCalledTimes(2)
  })
})
