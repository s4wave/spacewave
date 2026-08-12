import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { MessageInput } from './MessageInput.js'

afterEach(cleanup)

describe('MessageInput', () => {
  it('sends a trimmed message with Enter and clears the successful draft', async () => {
    const onSend = vi.fn(async () => {})
    render(<MessageInput onSend={onSend} />)
    const input = screen.getByRole('textbox', { name: 'Message' })
    fireEvent.change(input, { target: { value: '  hello channel  ' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    await waitFor(() => expect(onSend).toHaveBeenCalledWith('hello channel'))
    await waitFor(() => expect((input as HTMLTextAreaElement).value).toBe(''))
    expect(document.activeElement).toBe(input)
  })

  it('keeps a failed draft and lets the reader retry', async () => {
    const onSend = vi
      .fn()
      .mockRejectedValueOnce(new Error('Peer unavailable'))
      .mockResolvedValueOnce(undefined)
    render(<MessageInput onSend={onSend} />)
    const input = screen.getByRole('textbox', { name: 'Message' })
    fireEvent.change(input, { target: { value: 'still here' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send message' }))
    expect((await screen.findByRole('alert')).textContent).toContain(
      'Peer unavailable',
    )
    expect((input as HTMLTextAreaElement).value).toBe('still here')
    await waitFor(() => expect(document.activeElement).toBe(input))
    fireEvent.click(screen.getByRole('button', { name: 'Send message' }))
    await waitFor(() => expect(onSend).toHaveBeenCalledTimes(2))
    await waitFor(() => expect((input as HTMLTextAreaElement).value).toBe(''))
  })

  it('uses Shift+Enter for a newline and disables empty sends', () => {
    const onSend = vi.fn(async () => {})
    render(<MessageInput onSend={onSend} />)
    const input = screen.getByRole('textbox', { name: 'Message' })
    expect(
      (
        screen.getByRole('button', {
          name: 'Send message',
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true)
    fireEvent.change(input, { target: { value: 'line one' } })
    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
    expect(onSend).not.toHaveBeenCalled()
  })
})
