import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { StatusBar } from './StatusBar.js'

describe('StatusBar', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders "Frame: 1" text', () => {
    render(<StatusBar />)
    expect(screen.getByText('Frame: 1')).toBeDefined()
  })

  it('renders "Objects: 3" text', () => {
    render(<StatusBar />)
    expect(screen.getByText('Objects: 3')).toBeDefined()
  })

  it('renders "Editor" text', () => {
    render(<StatusBar />)
    expect(screen.getByText('Editor')).toBeDefined()
  })
})
