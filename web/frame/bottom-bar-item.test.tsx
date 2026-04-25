import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, fireEvent } from '@testing-library/react'
import { BottomBarItem } from './bottom-bar-item.js'

describe('BottomBarItem', () => {
  beforeEach(() => {
    cleanup()
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
