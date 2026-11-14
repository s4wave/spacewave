import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'

import { forgeBlockStatus, ForgeViewerShell } from './ForgeViewerShell.js'

// retry stands in for a resource's Retry.
const retry = () => {}

// block is a block resource in one read state.
function block(
  value: unknown,
  loading: boolean,
  error: Error | null = null,
): Resource<unknown> {
  return resourceOf({ value, loading, error, retry })
}

describe('forgeBlockStatus', () => {
  it.each([
    [
      'loading with no value',
      block(null, true),
      { state: 'waiting', what: 'the task' },
    ],
    ['loading with a value', block({ name: 't' }, true), null],
    [
      'failed',
      block(null, false, new Error('stream reset')),
      {
        state: 'failed',
        message: 'Could not read the task: stream reset',
        retry,
      },
    ],
    [
      'no block',
      block(null, false),
      { state: 'empty', message: 'No data is stored for the task yet.' },
    ],
    ['a value', block({ name: 't' }, false), null],
  ])('maps %s', (_, resource, want) => {
    expect(forgeBlockStatus('the task', resource)).toEqual(want)
  })
})

describe('ForgeViewerShell', () => {
  afterEach(cleanup)

  it('renders viewer bar actions separately from selection actions', async () => {
    // Count presses on each kind of action.
    const user = userEvent.setup()
    const createJob = vi.fn()
    const startWorker = vi.fn()

    // A dashboard with two tabs, a bar action and a selection action.
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
        headerActions={[{ id: 'job', label: 'Create Job', onClick: createJob }]}
        actions={[{ label: 'Start Worker', onClick: startWorker }]}
      />,
    )

    // Each action sits in its own bar.
    const header = screen.getByRole('banner')
    const actionBar = screen.getByTestId('forge-viewer-action-bar')
    expect(header.textContent).toContain('Create Job')
    expect(
      screen.getByRole('heading', { level: 2, name: 'Forge Dashboard' }),
    ).toBeTruthy()
    expect(actionBar.textContent).toContain('Start Worker')
    expect(header.textContent).not.toContain('Start Worker')
    expect(actionBar.textContent).not.toContain('Create Job')

    // The first tab shows in the centred column.
    expect(screen.getByText('Overview content')).toBeTruthy()
    expect(document.querySelectorAll('[class~="max-w-5xl"]').length).toBe(2)

    // Each press reaches its own handler once.
    await user.click(screen.getByRole('button', { name: 'Create Job' }))
    await user.click(screen.getByRole('button', { name: 'Start Worker' }))
    expect(createJob).toHaveBeenCalledTimes(1)
    expect(startWorker).toHaveBeenCalledTimes(1)
  })

  it('hides Details, the tabs and the actions while a status shows', () => {
    // A failed block takes the body; only the bar's title stays.
    render(
      <ForgeViewerShell
        icon={<span aria-hidden="true">F</span>}
        title="Task"
        stateKey="forge/task"
        details={{ label: 'Task', facts: <div>Task facts</div> }}
        status={{
          state: 'failed',
          message: 'Could not read the task: gone',
          retry,
        }}
        tabs={[
          {
            id: 'overview',
            label: 'Overview',
            content: <div>Overview content</div>,
          },
        ]}
        actions={[{ label: 'Start Worker', onClick: retry }]}
      />,
    )

    // The status shows in place of everything the block would fill.
    expect(screen.getByText('Could not read the task: gone')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Details' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Overview' })).toBeNull()
    expect(screen.queryByText('Overview content')).toBeNull()
    expect(screen.queryByTestId('forge-viewer-action-bar')).toBeNull()
  })
})
