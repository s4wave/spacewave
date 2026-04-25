import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CanvasContextMenu } from './CanvasContextMenu.js'

describe('CanvasContextMenu real interactions', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('fires the text-node callback when the real menu item is clicked', async () => {
    const user = userEvent.setup()
    const onAddText = vi.fn()

    render(
      <CanvasContextMenu
        state={{ position: { x: 10, y: 20 } }}
        canAddObject={true}
        onClose={vi.fn()}
        onPaste={vi.fn()}
        onAddText={onAddText}
        onAddObject={vi.fn()}
        onFitView={vi.fn()}
        onZoomReset={vi.fn()}
        onSelectAll={vi.fn()}
      />,
    )

    await user.click(screen.getByText('Add Text Node Here'))
    expect(onAddText).toHaveBeenCalledTimes(1)
  })

  it('still fires the callback when a controlled parent closes the menu', async () => {
    const user = userEvent.setup()
    const onAddText = vi.fn()

    function Wrapper() {
      const [state, setState] = React.useState<{
        position: { x: number; y: number }
      } | null>({
        position: { x: 10, y: 20 },
      })

      return (
        <CanvasContextMenu
          state={state}
          canAddObject={true}
          onClose={() => setState(null)}
          onPaste={vi.fn()}
          onAddText={onAddText}
          onAddObject={vi.fn()}
          onFitView={vi.fn()}
          onZoomReset={vi.fn()}
          onSelectAll={vi.fn()}
        />
      )
    }

    render(<Wrapper />)

    await user.click(screen.getByText('Add Text Node Here'))
    expect(onAddText).toHaveBeenCalledTimes(1)
  })
})
