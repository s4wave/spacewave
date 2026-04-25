import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CanvasTextNode } from './CanvasTextNode.js'

describe('CanvasTextNode', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders content in a pre element in view mode', () => {
    render(<CanvasTextNode content="Hello world" />)
    const pre = screen.getByText('Hello world')
    expect(pre.tagName).toBe('PRE')
  })

  it('preserves whitespace in view mode', () => {
    render(<CanvasTextNode content="  indented\n  text" />)
    const pre = screen.getByText('indented', { exact: false })
    expect(pre.className).toContain('whitespace-pre-wrap')
  })

  it('enters edit mode on double-click when onChange is provided', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<CanvasTextNode content="Edit me" onChange={onChange} />)

    const pre = screen.getByText('Edit me')
    await user.dblClick(pre)

    // Should now show a textarea.
    const textarea = document.querySelector('textarea')
    expect(textarea).toBeTruthy()
    expect(textarea?.value).toBe('Edit me')
  })

  it('does not enter edit mode on double-click without onChange', async () => {
    const user = userEvent.setup()
    render(<CanvasTextNode content="Read only" />)

    const pre = screen.getByText('Read only')
    await user.dblClick(pre)

    // Should still be a pre, not a textarea.
    const textarea = document.querySelector('textarea')
    expect(textarea).toBeFalsy()
  })

  it('commits edit on blur', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<CanvasTextNode content="Original" onChange={onChange} />)

    // Enter edit mode.
    const pre = screen.getByText('Original')
    await user.dblClick(pre)

    // Type new content.
    const textarea = document.querySelector('textarea') as HTMLTextAreaElement
    await user.clear(textarea)
    await user.type(textarea, 'Updated')

    // Blur to commit.
    fireEvent.blur(textarea)

    expect(onChange).toHaveBeenCalledWith('Updated')
  })

  it('cancels edit on Escape without calling onChange', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<CanvasTextNode content="Original" onChange={onChange} />)

    const pre = screen.getByText('Original')
    await user.dblClick(pre)

    const textarea = document.querySelector('textarea') as HTMLTextAreaElement
    await user.clear(textarea)
    await user.type(textarea, 'Changed')

    // Press Escape to cancel.
    await user.keyboard('{Escape}')

    // onChange should NOT be called (escape cancels).
    expect(onChange).not.toHaveBeenCalled()

    // Should return to view mode.
    expect(document.querySelector('textarea')).toBeFalsy()
    expect(screen.getByText('Original')).toBeTruthy()
  })

  it('commits edit on Ctrl+Enter', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<CanvasTextNode content="Original" onChange={onChange} />)

    const pre = screen.getByText('Original')
    await user.dblClick(pre)

    const textarea = document.querySelector('textarea') as HTMLTextAreaElement
    await user.clear(textarea)
    await user.type(textarea, 'New text')

    // Ctrl+Enter to commit.
    await user.keyboard('{Control>}{Enter}{/Control}')

    expect(onChange).toHaveBeenCalledWith('New text')
  })

  it('does not call onChange if content is unchanged', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<CanvasTextNode content="Same" onChange={onChange} />)

    const pre = screen.getByText('Same')
    await user.dblClick(pre)

    // Blur without changing.
    const textarea = document.querySelector('textarea') as HTMLTextAreaElement
    fireEvent.blur(textarea)

    expect(onChange).not.toHaveBeenCalled()
  })
})
