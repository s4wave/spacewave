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

const { downloadURL, toast } = vi.hoisted(() => ({
  downloadURL: vi.fn(),
  toast: { error: vi.fn() },
}))

vi.mock('@s4wave/web/download.js', () => ({ downloadURL }))
vi.mock('@s4wave/web/ui/toaster.js', () => ({ toast }))
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
    vi.clearAllMocks()
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

  it('provides immediate viewer selection feedback and an open affordance', () => {
    const onComponentSelect = vi.fn()
    const onCloseClick = vi.fn()
    render(
      <ObjectViewerDetails
        objectKey="glados/workfront/1"
        typeID="glados/workfront"
        availableComponents={components}
        selectedComponent={components[0]}
        onComponentSelect={onComponentSelect}
        onCloseClick={onCloseClick}
      />,
    )

    const debugRow = screen.getByRole('button', { name: /Debug Viewer/ })
    const workfrontRow = screen.getByRole('button', { name: /Workfront/ })
    expect(debugRow.getAttribute('aria-pressed')).toBe('true')
    expect(workfrontRow.getAttribute('aria-pressed')).toBe('false')
    expect(debugRow.className).toContain('border-brand/30')
    expect(screen.getByText('Active')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Open viewer' })).toBeDefined()

    fireEvent.click(workfrontRow)

    expect(onComponentSelect).toHaveBeenCalledWith(components[1])
    expect(workfrontRow.getAttribute('aria-pressed')).toBe('true')
    expect(debugRow.getAttribute('aria-pressed')).toBe('false')
    expect(screen.getByRole('button', { name: 'Open viewer' })).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Open viewer' }))
    expect(onCloseClick).toHaveBeenCalledTimes(1)
  })

  it('persists export busy and user-safe failure states', async () => {
    let rejectExport!: (reason: unknown) => void
    downloadURL.mockReturnValueOnce(
      new Promise<void>((_, reject) => {
        rejectExport = reject
      }),
    )

    render(
      <ObjectViewerDetails
        objectKey="glados/workfront/1"
        typeID="glados/workfront"
        exportUrl="/exports/glados-workfront-1.zip"
        availableComponents={[]}
        onComponentSelect={vi.fn()}
      />,
    )

    const exportButton = screen.getByRole('button', { name: /Export Data/ })
    fireEvent.click(exportButton)

    expect(screen.getByText('Preparing export…')).toBeDefined()
    expect(exportButton.getAttribute('disabled')).not.toBeNull()

    rejectExport(new Error('internal export status 503'))
    await waitFor(() => {
      expect(screen.getByText('Export could not be prepared')).toBeDefined()
    })

    expect(screen.queryByText('internal export status 503')).toBeNull()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeDefined()
    expect(toast.error).toHaveBeenCalledWith('Export could not be prepared')
  })

  it('persists a successful export state', async () => {
    downloadURL.mockResolvedValueOnce(undefined)

    render(
      <ObjectViewerDetails
        objectKey="glados/workfront/1"
        typeID="glados/workfront"
        exportUrl="/exports/glados-workfront-1.zip"
        availableComponents={[]}
        onComponentSelect={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Export Data/ }))

    await waitFor(() => {
      expect(screen.getByText('Export ready')).toBeDefined()
    })
    expect(
      screen.getByText('The object contents are ready to download.'),
    ).toBeDefined()
  })
})
