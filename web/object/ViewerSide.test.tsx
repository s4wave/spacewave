import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { ViewerSide } from './ViewerSide.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'

const facts = <FactRow label="Setup" value="Ready" kind="text" control={null} />

describe('ViewerSide', () => {
  afterEach(() => {
    cleanup()
  })

  it('keeps focus on the press that replaces Collapse or Expand', () => {
    // Show the open column.
    render(<ViewerSide label="Device" facts={facts} />)

    // Collapse to the rail and see focus land on Expand.
    fireEvent.click(screen.getByRole('button', { name: 'Collapse device' }))
    const expand = screen.getByRole('button', { name: 'Expand device' })
    expect(document.activeElement).toBe(expand)
    expect(screen.queryByText('Ready')).toBeNull()

    // Expand again and see the facts and focus on Collapse.
    fireEvent.click(expand)
    expect(document.activeElement).toBe(
      screen.getByRole('button', { name: 'Collapse device' }),
    )
    expect(screen.getByText('Ready')).toBeTruthy()
  })
})
