import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ForgeViewerShell } from './ForgeViewerShell.js'

describe('ForgeViewerShell', () => {
  it('renders global header actions separately from selection actions', async () => {
    const user = userEvent.setup()
    const createJob = vi.fn()
    const startWorker = vi.fn()

    render(
      <ForgeViewerShell
        icon={<span aria-hidden="true">F</span>}
        title="Forge Dashboard"
        stateKey="forge/dashboard"
        tabs={[
          {
            id: 'overview',
            label: 'Overview',
            content: <div>Overview content</div>,
          },
          {
            id: 'activity',
            label: 'Activity',
            content: <div>Activity content</div>,
          },
        ]}
        headerActions={[{ label: 'Create Job', onClick: createJob }]}
        actions={[{ label: 'Start Worker', onClick: startWorker }]}
      />,
    )

    const header = screen.getByTestId('forge-viewer-header-actions')
    const actionBar = screen.getByTestId('forge-viewer-action-bar')
    expect(header.textContent).toContain('Create Job')
    expect(actionBar.textContent).toContain('Start Worker')
    expect(header.textContent).not.toContain('Start Worker')
    expect(actionBar.textContent).not.toContain('Create Job')
    expect(screen.getByText('Overview content')).toBeTruthy()
    expect(document.querySelectorAll('[class~="max-w-5xl"]').length).toBe(2)

    await user.click(screen.getByRole('button', { name: 'Create Job' }))
    await user.click(screen.getByRole('button', { name: 'Start Worker' }))
    expect(createJob).toHaveBeenCalledTimes(1)
    expect(startWorker).toHaveBeenCalledTimes(1)
  })
})
