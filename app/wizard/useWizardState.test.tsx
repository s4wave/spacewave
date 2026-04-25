import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useWizardState } from './useWizardState.js'

const h = vi.hoisted(() => ({
  updateState: vi.fn().mockResolvedValue({}),
}))

let currentConfigData = new Uint8Array()

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: {
      updateState: h.updateState,
    },
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: {
      step: 0,
      targetTypeId: 'git/repo',
      targetKeyPrefix: 'git/repo/',
      name: 'Repo',
      configData: currentConfigData,
    },
  }),
}))

vi.mock('@s4wave/web/configtype/useConfigEditor.js', () => ({
  useConfigEditor: (
    _configTypeId: string,
    configData: Uint8Array | undefined,
    onConfigDataChange: (data: Uint8Array) => void,
  ) => {
    const value = new TextDecoder().decode(configData ?? new Uint8Array())
    return {
      element: (
        <input
          aria-label="Config value"
          value={value}
          onChange={(e) =>
            onConfigDataChange(new TextEncoder().encode(e.target.value))
          }
        />
      ),
      registration: undefined,
      value,
    }
  },
}))

describe('useWizardState', () => {
  afterEach(() => {
    currentConfigData = new Uint8Array()
    cleanup()
    vi.clearAllMocks()
  })

  it('keeps config edits local until the draft is persisted', async () => {
    const user = userEvent.setup()
    renderHarness()

    await user.type(screen.getByLabelText('Config value'), 'abc')

    expect(h.updateState).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /persist/i }))

    await waitFor(() => {
      expect(h.updateState).toHaveBeenCalledTimes(1)
    })
    expect(h.updateState).toHaveBeenCalledWith({
      configData: new TextEncoder().encode('abc'),
    })
  })
})

function renderHarness() {
  return render(
    <SpaceContainerContext.Provider
      spaceId="space-1"
      spaceState={{ ready: true }}
      spaceWorldResource={{
        value: { deleteObject: vi.fn() } as never,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
      spaceWorld={{ deleteObject: vi.fn() } as never}
      navigateToRoot={vi.fn()}
      navigateToObjects={vi.fn()}
      buildObjectUrls={vi.fn()}
      navigateToSubPath={vi.fn()}
    >
      <Harness />
    </SpaceContainerContext.Provider>,
  )
}

function Harness() {
  const ws = useWizardState(
    {
      objectInfo: {
        info: {
          case: 'worldObjectInfo',
          value: {
            objectKey: 'wizard/git/repo/test',
            objectType: 'wizard/git/repo',
          },
        },
      },
      worldState: {
        value: {} as never,
        loading: false,
        error: null,
        retry: vi.fn(),
      },
    },
    'git/repo',
  )
  return (
    <>
      {ws.configEditor.element}
      <button onClick={() => void ws.persistDraftState()}>Persist</button>
    </>
  )
}
