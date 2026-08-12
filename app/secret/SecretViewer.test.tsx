import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SharedObjectHealthStatus } from '@s4wave/core/sobject/sobject.pb.js'
import type { SecretState } from '@s4wave/sdk/secret/secret.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { SecretViewer } from './SecretViewer.js'

const secretValue = 'production-token-must-never-render'
const { hookState } = vi.hoisted(() => ({
  hookState: {
    value: null as SecretState | null,
    loading: false,
    error: null as Error | null,
    retry: vi.fn(),
  },
}))

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({ value: {}, loading: false, error: null }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => hookState,
}))

function worldState(): Resource<IWorldState> {
  return {
    value: {} as IWorldState,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

function renderViewer() {
  return render(
    <SecretViewer
      objectInfo={{
        info: {
          case: 'worldObjectInfo',
          value: {
            objectKey: 'credentials/matrix-bot',
            objectType: 'spacewave/secret',
          },
        },
      }}
      worldState={worldState()}
    />,
  )
}

function readyState(readable: boolean): SecretState {
  return {
    secret: {
      displayName: 'Matrix bot token',
      kind: 'matrix_access_token',
      nestedSharedObjectId: 'shared-object-redacted-ref',
      createdAt: new Date('2026-06-01T10:00:00Z'),
      updatedAt: new Date('2026-06-02T11:30:00Z'),
    },
    grantStatus: {
      participant: readable,
      readable,
      grantCount: readable ? 1 : 0,
    },
    health: { status: SharedObjectHealthStatus.READY },
    payload: { value: new TextEncoder().encode(secretValue) },
  } as SecretState
}

describe('SecretViewer', () => {
  beforeEach(() => {
    hookState.value = null
    hookState.loading = false
    hookState.error = null
    hookState.retry.mockClear()
  })
  afterEach(cleanup)

  it('keeps access unknown while the nested SharedObject is loading', () => {
    hookState.value = {
      health: { status: SharedObjectHealthStatus.LOADING },
      grantStatus: { participant: false, readable: false, grantCount: 0 },
    }
    renderViewer()

    expect(screen.getByText('Nested SharedObject status')).toBeDefined()
    expect(screen.getByText('Loading')).toBeDefined()
    expect(screen.queryByText('Not readable')).toBeNull()
    expect(screen.queryByText('Not granted')).toBeNull()
    expect(screen.queryByText('Active grants')).toBeNull()
  })

  it.each([
    [false, 'Not readable', 'Not granted', '0'],
    [true, 'Readable', 'Granted', '1'],
  ] as const)(
    'renders exact ready access state when readability is %s',
    (readable, readability, participant, grants) => {
      hookState.value = readyState(readable)
      renderViewer()

      expect(screen.getByText('Nested SharedObject readability')).toBeDefined()
      expect(screen.getByText(readability)).toBeDefined()
      expect(screen.getByText(participant)).toBeDefined()
      expect(screen.getByText(grants)).toBeDefined()
      expect(screen.getAllByText('Matrix bot token')).toHaveLength(2)
      expect(screen.getByText('••••••••••••')).toBeDefined()
      expect(screen.queryByText(secretValue)).toBeNull()
      expect(screen.queryByRole('button', { name: /reveal/i })).toBeNull()
      expect(screen.getByText('Nested SharedObject ID')).toBeDefined()
    },
  )

  it('shows retryable unavailable state when the stream completes without state', () => {
    renderViewer()

    expect(screen.getByText('Secret metadata unavailable')).toBeDefined()
    expect(
      screen.getByText('The Secret resource returned no state.'),
    ).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(hookState.retry).toHaveBeenCalledTimes(1)
  })

  it('hides stale metadata behind a retryable stream error', () => {
    hookState.value = readyState(true)
    hookState.error = new Error('stream failed')
    renderViewer()

    expect(screen.getByText('Secret metadata unavailable')).toBeDefined()
    expect(screen.queryByText('Matrix bot token')).toBeNull()
    expect(screen.queryByText('stream failed')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(hookState.retry).toHaveBeenCalledTimes(1)
  })
})
