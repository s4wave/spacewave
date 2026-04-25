import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SearchBox } from './SearchBox.js'

describe('SearchBox', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows "Open search" button initially when not focused', () => {
    render(<SearchBox />)
    const button = screen.getByRole('button', { name: 'Open search' })
    expect(button).toBeTruthy()
  })

  it('expands to show input when "Open search" button is clicked', async () => {
    const user = userEvent.setup()
    render(<SearchBox />)

    const button = screen.getByRole('button', { name: 'Open search' })
    await user.click(button)

    const input = screen.getByRole('textbox')
    expect(input).toBeTruthy()
  })

  it('shows input immediately when autoFocus is true', () => {
    render(<SearchBox autoFocus />)

    const input = screen.getByRole('textbox')
    expect(input).toBeTruthy()
  })

  it('input has default placeholder text "Search"', async () => {
    const user = userEvent.setup()
    render(<SearchBox />)

    const button = screen.getByRole('button', { name: 'Open search' })
    await user.click(button)

    const input = screen.getByPlaceholderText('Search')
    expect(input).toBeTruthy()
  })

  it('input has custom placeholder text', async () => {
    const user = userEvent.setup()
    render(<SearchBox placeholder="Find files..." />)

    const button = screen.getByRole('button', { name: 'Open search' })
    await user.click(button)

    const input = screen.getByPlaceholderText('Find files...')
    expect(input).toBeTruthy()
  })

  it('calls onSearch when Enter is pressed with non-empty query', async () => {
    const user = userEvent.setup()
    const onSearch = vi.fn()
    render(<SearchBox onSearch={onSearch} autoFocus />)

    const input = screen.getByRole('textbox')
    await user.type(input, 'hello{Enter}')

    expect(onSearch).toHaveBeenCalledWith('hello')
  })

  it('does not call onSearch when Enter is pressed with empty query', async () => {
    const onSearch = vi.fn()
    render(<SearchBox onSearch={onSearch} autoFocus />)

    screen.getByRole('textbox')
    await userEvent.setup().keyboard('{Enter}')

    expect(onSearch).not.toHaveBeenCalled()
  })

  it('calls onSearch when input blurs with non-empty query', async () => {
    const onSearch = vi.fn()
    render(<SearchBox onSearch={onSearch} autoFocus />)

    const input = screen.getByRole('textbox')
    await userEvent.setup().type(input, 'test query')
    input.blur()

    await waitFor(() => {
      expect(onSearch).toHaveBeenCalledWith('test query')
    })
  })

  it('calls onBlur when input blurs', async () => {
    const onBlur = vi.fn()
    render(<SearchBox onBlur={onBlur} autoFocus />)

    const input = screen.getByRole('textbox')
    input.blur()

    await waitFor(() => {
      expect(onBlur).toHaveBeenCalled()
    })
  })

  it('does not call onSearch when blur with empty query', async () => {
    const onSearch = vi.fn()
    render(<SearchBox onSearch={onSearch} autoFocus />)

    const input = screen.getByRole('textbox')
    input.blur()

    await waitFor(() => {
      expect(onSearch).not.toHaveBeenCalled()
    })
  })
})
