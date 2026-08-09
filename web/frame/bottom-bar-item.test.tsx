import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, fireEvent } from '@testing-library/react'
import { BottomBarItem } from './bottom-bar-item.js'

describe('BottomBarItem', () => {
  beforeEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  describe('Keyboard Accessibility', () => {
    it('calls onClick when Space key is pressed', () => {
      const onClick = vi.fn()
      const { container } = render(
        <BottomBarItem onClick={onClick}>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.keyDown(item, { key: ' ' })

      expect(onClick).toHaveBeenCalledTimes(1)
    })

    it('calls onClick when Enter key is pressed', () => {
      const onClick = vi.fn()
      const { container } = render(
        <BottomBarItem onClick={onClick}>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.keyDown(item, { key: 'Enter' })

      expect(onClick).toHaveBeenCalledTimes(1)
    })

    it('calls preventDefault on Space key to avoid scrolling', () => {
      const onClick = vi.fn()
      const { container } = render(
        <BottomBarItem onClick={onClick}>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement

      const event = new KeyboardEvent('keydown', {
        key: ' ',
        bubbles: true,
        cancelable: true,
      })
      item.dispatchEvent(event)

      expect(event.defaultPrevented).toBe(true)
      expect(onClick).toHaveBeenCalledTimes(1)
    })

    it('does not call onClick when other keys are pressed', () => {
      const onClick = vi.fn()
      const { container } = render(
        <BottomBarItem onClick={onClick}>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.keyDown(item, { key: 'a' })
      fireEvent.keyDown(item, { key: 'Escape' })
      fireEvent.keyDown(item, { key: 'Tab' })

      expect(onClick).not.toHaveBeenCalled()
    })

    it('opens secondary actions with the ContextMenu key', () => {
      const onClick = vi.fn()
      const onSecondaryActivate = vi.fn()
      const { container } = render(
        <BottomBarItem
          onClick={onClick}
          onSecondaryActivate={onSecondaryActivate}
        >
          Test Item
        </BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      item.getBoundingClientRect = () => ({
        left: 10,
        top: 20,
        width: 80,
        height: 20,
        right: 90,
        bottom: 40,
        x: 10,
        y: 20,
        toJSON: () => {},
      })
      fireEvent.keyDown(item, { key: 'ContextMenu' })

      expect(onClick).not.toHaveBeenCalled()
      expect(onSecondaryActivate).toHaveBeenCalledWith({
        openKind: 'keyboard',
        x: 50,
        y: 20,
        trigger: item,
      })
    })

    it('opens secondary actions with Shift+F10', () => {
      const onClick = vi.fn()
      const onSecondaryActivate = vi.fn()
      const { container } = render(
        <BottomBarItem
          onClick={onClick}
          onSecondaryActivate={onSecondaryActivate}
        >
          Test Item
        </BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.keyDown(item, { key: 'F10', shiftKey: true })

      expect(onClick).not.toHaveBeenCalled()
      expect(onSecondaryActivate).toHaveBeenCalledWith(
        expect.objectContaining({ openKind: 'keyboard', trigger: item }),
      )
    })

    it('keeps Enter and Space on the primary click path when secondary actions exist', () => {
      const onClick = vi.fn()
      const onSecondaryActivate = vi.fn()
      const { container } = render(
        <BottomBarItem
          onClick={onClick}
          onSecondaryActivate={onSecondaryActivate}
        >
          Test Item
        </BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.keyDown(item, { key: 'Enter' })
      fireEvent.keyDown(item, { key: ' ' })

      expect(onClick).toHaveBeenCalledTimes(2)
      expect(onSecondaryActivate).not.toHaveBeenCalled()
    })

    it('does not call onClick when onClick is undefined', () => {
      const { container } = render(<BottomBarItem>Test Item</BottomBarItem>)

      const item = container.firstChild as HTMLElement

      expect(() => {
        fireEvent.keyDown(item, { key: ' ' })
        fireEvent.keyDown(item, { key: 'Enter' })
      }).not.toThrow()
    })

    it('has correct ARIA attributes for button role', () => {
      const { container } = render(
        <BottomBarItem onClick={() => {}}>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      expect(item.getAttribute('role')).toBe('button')
      expect(item.getAttribute('tabIndex')).toBe('0')
    })

    it('applies aria-disabled when disabled prop is true', () => {
      const { container } = render(
        <BottomBarItem disabled>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      expect(item.getAttribute('aria-disabled')).toBe('true')
    })

    it('applies aria-selected when selected prop is true', () => {
      const { container } = render(
        <BottomBarItem selected>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      expect(item.getAttribute('aria-selected')).toBe('true')
    })

    it('applies menu ARIA state when secondary actions are available', () => {
      const { container, rerender } = render(
        <BottomBarItem onSecondaryActivate={() => {}}>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      expect(item.getAttribute('aria-haspopup')).toBe('menu')
      expect(item.getAttribute('aria-expanded')).toBe('false')

      rerender(
        <BottomBarItem onSecondaryActivate={() => {}} contextMenuOpen>
          Test Item
        </BottomBarItem>,
      )
      expect(item.getAttribute('aria-expanded')).toBe('true')
    })
  })

  describe('Click Handling', () => {
    it('calls onClick when clicked with mouse', () => {
      const onClick = vi.fn()
      const { container } = render(
        <BottomBarItem onClick={onClick}>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.click(item)

      expect(onClick).toHaveBeenCalledTimes(1)
    })

    it('opens secondary actions from right-click without primary click', () => {
      const onClick = vi.fn()
      const onSecondaryActivate = vi.fn()
      const { container } = render(
        <BottomBarItem
          onClick={onClick}
          onSecondaryActivate={onSecondaryActivate}
        >
          Test Item
        </BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.contextMenu(item, { clientX: 12, clientY: 34 })

      expect(onClick).not.toHaveBeenCalled()
      expect(onSecondaryActivate).toHaveBeenCalledWith({
        openKind: 'mouse',
        x: 12,
        y: 34,
        trigger: item,
      })
    })

    it('opens secondary actions from long-press and suppresses the follow-up click', () => {
      vi.useFakeTimers()
      const onClick = vi.fn()
      const onSecondaryActivate = vi.fn()
      const { container } = render(
        <BottomBarItem
          onClick={onClick}
          onSecondaryActivate={onSecondaryActivate}
        >
          Test Item
        </BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.pointerDown(item, {
        pointerId: 1,
        pointerType: 'touch',
        clientX: 12,
        clientY: 34,
      })
      vi.advanceTimersByTime(550)
      fireEvent.click(item)

      expect(onSecondaryActivate).toHaveBeenCalledWith({
        openKind: 'touch',
        x: 12,
        y: 34,
        trigger: item,
      })
      expect(onClick).not.toHaveBeenCalled()

      fireEvent.click(item)
      expect(onClick).toHaveBeenCalledTimes(1)
    })

    it('cancels long-press when the pointer moves beyond the tolerance', () => {
      vi.useFakeTimers()
      const onSecondaryActivate = vi.fn()
      const { container } = render(
        <BottomBarItem onSecondaryActivate={onSecondaryActivate}>
          Test Item
        </BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.pointerDown(item, {
        pointerId: 1,
        pointerType: 'touch',
        clientX: 12,
        clientY: 34,
      })
      fireEvent.pointerMove(item, {
        pointerId: 1,
        pointerType: 'touch',
        clientX: 25,
        clientY: 34,
      })
      vi.advanceTimersByTime(550)

      expect(onSecondaryActivate).not.toHaveBeenCalled()
    })

    it('cancels long-press when the pointer lifts before the delay', () => {
      vi.useFakeTimers()
      const onSecondaryActivate = vi.fn()
      const { container } = render(
        <BottomBarItem onSecondaryActivate={onSecondaryActivate}>
          Test Item
        </BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      fireEvent.pointerDown(item, {
        pointerId: 1,
        pointerType: 'touch',
        clientX: 12,
        clientY: 34,
      })
      fireEvent.pointerUp(item, { pointerId: 1, pointerType: 'touch' })
      vi.advanceTimersByTime(550)

      expect(onSecondaryActivate).not.toHaveBeenCalled()
    })
  })

  describe('Styling', () => {
    it('reflects selected state via aria-selected attribute', () => {
      const { container, rerender } = render(
        <BottomBarItem selected>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      expect(item.getAttribute('aria-selected')).toBe('true')

      rerender(<BottomBarItem>Test Item</BottomBarItem>)
      expect(item.getAttribute('aria-selected')).toBeNull()
    })

    it('applies custom className', () => {
      const { container } = render(
        <BottomBarItem className="custom-class">Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      expect(item.className).toContain('custom-class')
    })

    it('sets cursor to not-allowed when disabled', () => {
      const { container } = render(
        <BottomBarItem disabled>Test Item</BottomBarItem>,
      )

      const item = container.firstChild as HTMLElement
      expect(item.style.cursor).toBe('not-allowed')
    })

    it('sets cursor to pointer when not disabled', () => {
      const { container } = render(<BottomBarItem>Test Item</BottomBarItem>)

      const item = container.firstChild as HTMLElement
      expect(item.style.cursor).toBe('pointer')
    })
  })

  describe('Children Rendering', () => {
    it('renders children content', () => {
      const { getByText } = render(
        <BottomBarItem>
          <span>Child Content</span>
        </BottomBarItem>,
      )

      expect(getByText('Child Content')).toBeTruthy()
    })
  })
})
