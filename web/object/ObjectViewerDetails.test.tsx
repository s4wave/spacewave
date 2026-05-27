import { cleanup, render, screen } from '@testing-library/react'
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
})
