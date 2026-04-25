import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { OptionalPinLock } from './OptionalPinLock.js'

describe('OptionalPinLock', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the optional PIN label, both inputs, and the input id', () => {
    render(
      <OptionalPinLock
        pin=""
        confirmPin=""
        pinError=""
        onPinChange={vi.fn()}
        onConfirmPinChange={vi.fn()}
        onSubmit={vi.fn()}
        disabled={false}
        pinInputId="test-pin"
      />,
    )

    expect(screen.getByText('Optional PIN lock')).toBeDefined()
    expect(screen.getByPlaceholderText('Leave blank to skip')).toBeDefined()
    expect(screen.getByPlaceholderText('Confirm PIN')).toBeDefined()
    expect(
      (screen.getByPlaceholderText('Leave blank to skip') as HTMLInputElement)
        .id,
    ).toBe('test-pin')
  })

  it('reports PIN and confirm-PIN changes through their callbacks', () => {
    const onPinChange = vi.fn()
    const onConfirmPinChange = vi.fn()

    render(
      <OptionalPinLock
        pin=""
        confirmPin=""
        pinError=""
        onPinChange={onPinChange}
        onConfirmPinChange={onConfirmPinChange}
        onSubmit={vi.fn()}
        disabled={false}
        pinInputId="test-pin"
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('Leave blank to skip'), {
      target: { value: 'hunter2' },
    })
    fireEvent.change(screen.getByPlaceholderText('Confirm PIN'), {
      target: { value: 'hunter2' },
    })

    expect(onPinChange).toHaveBeenCalledWith('hunter2')
    expect(onConfirmPinChange).toHaveBeenCalledWith('hunter2')
  })

  it('renders the error row only when pinError is set', () => {
    const { rerender } = render(
      <OptionalPinLock
        pin=""
        confirmPin=""
        pinError=""
        onPinChange={vi.fn()}
        onConfirmPinChange={vi.fn()}
        onSubmit={vi.fn()}
        disabled={false}
        pinInputId="test-pin"
      />,
    )

    expect(screen.queryByText('PINs do not match')).toBeNull()

    rerender(
      <OptionalPinLock
        pin="a"
        confirmPin="b"
        pinError="PINs do not match"
        onPinChange={vi.fn()}
        onConfirmPinChange={vi.fn()}
        onSubmit={vi.fn()}
        disabled={false}
        pinInputId="test-pin"
      />,
    )

    expect(screen.getByText('PINs do not match')).toBeDefined()
  })

  it('invokes onSubmit when Enter is pressed and disabled is false', () => {
    const onSubmit = vi.fn()

    render(
      <OptionalPinLock
        pin=""
        confirmPin=""
        pinError=""
        onPinChange={vi.fn()}
        onConfirmPinChange={vi.fn()}
        onSubmit={onSubmit}
        disabled={false}
        pinInputId="test-pin"
      />,
    )

    fireEvent.keyDown(screen.getByPlaceholderText('Leave blank to skip'), {
      key: 'Enter',
    })
    fireEvent.keyDown(screen.getByPlaceholderText('Confirm PIN'), {
      key: 'Enter',
    })

    expect(onSubmit).toHaveBeenCalledTimes(2)
  })

  it('does not invoke onSubmit when disabled is true', () => {
    const onSubmit = vi.fn()

    render(
      <OptionalPinLock
        pin=""
        confirmPin=""
        pinError=""
        onPinChange={vi.fn()}
        onConfirmPinChange={vi.fn()}
        onSubmit={onSubmit}
        disabled={true}
        pinInputId="test-pin"
      />,
    )

    fireEvent.keyDown(screen.getByPlaceholderText('Leave blank to skip'), {
      key: 'Enter',
    })

    expect(onSubmit).not.toHaveBeenCalled()
  })
})
