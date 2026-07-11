import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ObjectViewerComponent } from './object.js'
import { ObjectViewerDetails } from './ObjectViewerDetails.js'

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}))

const components: ObjectViewerComponent[] = [
  {
    componentID: 'spacewave.debug.viewer',
    typeID: '*',
    name: 'Debug Viewer',
    component: () => null,
  },
  {
    componentID: 'glados.workfront.viewer',
    typeID: 'glados/workfront',
    name: 'Workfront',
    component: () => null,
  },
]

describe('ObjectViewerDetails', () => {
  afterEach(() => {
    cleanup()
  })

  it('exposes missing requested component IDs in the object internals panel', () => {
    render(
      <ObjectViewerDetails
        objectKey="glados/workfront/1"
        typeID="glados/workfront"
        availableComponents={components}
        selectedComponent={components[0]}
        missingComponentID="glados.missing.viewer"
        onComponentSelect={vi.fn()}
      />,
    )

    expect(screen.getByText('Missing Component ID')).toBeDefined()
    expect(screen.getByText('glados.missing.viewer')).toBeDefined()
    expect(screen.getByText('ID: spacewave.debug.viewer')).toBeDefined()
  })

  it('omits the danger zone when object deletion is unavailable', () => {
    render(
      <ObjectViewerDetails
        objectKey="glados/workfront/1"
        typeID="glados/workfront"
        availableComponents={components}
        onComponentSelect={vi.fn()}
      />,
    )

    expect(screen.queryByText('Danger Zone')).toBeNull()
  })

  it('names the object before confirming deletion', async () => {
    const onDeleteConfirm = vi.fn().mockResolvedValue(undefined)
    render(
      <ObjectViewerDetails
        objectKey="glados/workfront/1"
        typeID="glados/workfront"
        availableComponents={components}
        onComponentSelect={vi.fn()}
        onDeleteConfirm={onDeleteConfirm}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Danger Zone' }))
    fireEvent.click(screen.getByRole('button', { name: /Delete Object/ }))

    const confirmation = screen.getByText(/This will permanently delete/)
    expect(confirmation.textContent).toContain('glados/workfront/1')

    const deleteButtons = screen.getAllByRole('button', {
      name: 'Delete Object',
    })
    fireEvent.click(deleteButtons.at(-1)!)

    await waitFor(() => {
      expect(onDeleteConfirm).toHaveBeenCalledTimes(1)
    })
  })
})
