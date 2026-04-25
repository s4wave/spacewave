import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CanvasToolbar } from './CanvasToolbar.js'
import type { CanvasAction } from './types.js'

function makeActions(): Record<CanvasAction, () => void> {
  return {
    delete: vi.fn(),
    copy: vi.fn(),
    paste: vi.fn(),
    undo: vi.fn(),
    redo: vi.fn(),
    'select-all': vi.fn(),
    deselect: vi.fn(),
    'zoom-in': vi.fn(),
    'zoom-out': vi.fn(),
    'zoom-reset': vi.fn(),
    'fit-view': vi.fn(),
    'bring-to-front': vi.fn(),
    'send-to-back': vi.fn(),
  }
}

describe('CanvasToolbar', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders all four tool buttons', () => {
    const onToolChange = vi.fn()
    render(
      <CanvasToolbar
        tool="select"
        onToolChange={onToolChange}
        actions={makeActions()}
      />,
    )
    expect(screen.getByLabelText('Select (V)')).toBeTruthy()
    expect(screen.getByLabelText('Draw (D)')).toBeTruthy()
    expect(screen.getByLabelText('Text (T)')).toBeTruthy()
    expect(screen.getByLabelText('Object (O)')).toBeTruthy()
  })

  it('renders action buttons for zoom and fit', () => {
    render(
      <CanvasToolbar
        tool="select"
        onToolChange={vi.fn()}
        actions={makeActions()}
      />,
    )
    expect(screen.getByLabelText('Zoom In (+)')).toBeTruthy()
    expect(screen.getByLabelText('Zoom Out (-)')).toBeTruthy()
    expect(screen.getByLabelText('Fit View')).toBeTruthy()
  })

  it('calls onToolChange with correct tool when buttons are clicked', async () => {
    const user = userEvent.setup()
    const onToolChange = vi.fn()
    render(
      <CanvasToolbar
        tool="select"
        onToolChange={onToolChange}
        actions={makeActions()}
      />,
    )

    await user.click(screen.getByLabelText('Draw (D)'))
    expect(onToolChange).toHaveBeenCalledWith('draw')

    await user.click(screen.getByLabelText('Text (T)'))
    expect(onToolChange).toHaveBeenCalledWith('text')

    await user.click(screen.getByLabelText('Object (O)'))
    expect(onToolChange).toHaveBeenCalledWith('object')

    await user.click(screen.getByLabelText('Select (V)'))
    expect(onToolChange).toHaveBeenCalledWith('select')
  })

  it('calls zoom-in action when zoom in button is clicked', async () => {
    const user = userEvent.setup()
    const actions = makeActions()
    render(
      <CanvasToolbar tool="select" onToolChange={vi.fn()} actions={actions} />,
    )
    await user.click(screen.getByLabelText('Zoom In (+)'))
    expect(actions['zoom-in']).toHaveBeenCalled()
  })

  it('calls zoom-out action when zoom out button is clicked', async () => {
    const user = userEvent.setup()
    const actions = makeActions()
    render(
      <CanvasToolbar tool="select" onToolChange={vi.fn()} actions={actions} />,
    )
    await user.click(screen.getByLabelText('Zoom Out (-)'))
    expect(actions['zoom-out']).toHaveBeenCalled()
  })

  it('calls fit-view action when fit view button is clicked', async () => {
    const user = userEvent.setup()
    const actions = makeActions()
    render(
      <CanvasToolbar tool="select" onToolChange={vi.fn()} actions={actions} />,
    )
    await user.click(screen.getByLabelText('Fit View'))
    expect(actions['fit-view']).toHaveBeenCalled()
  })

  it('highlights the active tool button', () => {
    render(
      <CanvasToolbar
        tool="draw"
        onToolChange={vi.fn()}
        actions={makeActions()}
      />,
    )
    const drawBtn = screen.getByLabelText('Draw (D)')
    expect(drawBtn.className).toContain('bg-foreground/10')

    const selectBtn = screen.getByLabelText('Select (V)')
    expect(selectBtn.className).not.toContain('bg-foreground/10')
  })

  it('renders a separator between tool and action buttons', () => {
    render(
      <CanvasToolbar
        tool="select"
        onToolChange={vi.fn()}
        actions={makeActions()}
      />,
    )
    // Separator is a div with bg-foreground/6 class.
    const separator = document.querySelector('.bg-foreground\\/6')
    expect(separator).toBeTruthy()
  })

  it('hides the Add Existing Object button when onAddObject is not provided', () => {
    render(
      <CanvasToolbar
        tool="select"
        onToolChange={vi.fn()}
        actions={makeActions()}
      />,
    )
    expect(screen.queryByLabelText('Add Existing Object')).toBeNull()
  })

  it('renders the Add Existing Object button when onAddObject is provided', () => {
    render(
      <CanvasToolbar
        tool="select"
        onToolChange={vi.fn()}
        actions={makeActions()}
        onAddObject={vi.fn()}
      />,
    )
    expect(screen.getByLabelText('Add Existing Object')).toBeTruthy()
  })

  it('calls onAddObject when the Add Existing Object button is clicked', async () => {
    const user = userEvent.setup()
    const onAddObject = vi.fn()
    render(
      <CanvasToolbar
        tool="select"
        onToolChange={vi.fn()}
        actions={makeActions()}
        onAddObject={onAddObject}
      />,
    )
    await user.click(screen.getByLabelText('Add Existing Object'))
    expect(onAddObject).toHaveBeenCalledTimes(1)
  })
})
