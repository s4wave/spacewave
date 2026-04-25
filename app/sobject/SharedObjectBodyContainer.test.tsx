import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { MountSharedObjectResponse } from '@s4wave/sdk/session/session.pb.js'
import { SharedObject } from '@s4wave/sdk/sobject/sobject.js'

import { SharedObjectBodyContainer } from './SharedObjectBodyContainer.js'

vi.mock('@s4wave/app/space/SpaceContainer.js', () => ({
  SpaceContainer: () => <div data-testid="space-container" />,
}))

function sharedObjectWithBodyType(bodyType: string | undefined): SharedObject {
  const meta: MountSharedObjectResponse = {
    sharedObjectId: 'so-id',
    blockStoreId: 'bs-id',
    peerId: 'peer-id',
    sharedObjectMeta: bodyType ? { bodyType } : undefined,
  }
  return {
    meta,
    resourceRef: { resourceId: 1, released: false },
    id: 1,
    client: {},
    service: {},
    mountSharedObjectBody: vi.fn(),
  } as unknown as SharedObject
}

function renderContainer(sharedObject: SharedObject | null) {
  return render(
    <SharedObjectContext.Provider
      resource={{
        value: sharedObject,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    >
      <SharedObjectBodyContainer />
    </SharedObjectContext.Provider>,
  )
}

describe('SharedObjectBodyContainer', () => {
  beforeEach(() => {
    cleanup()
  })

  it('renders SpaceContainer for the "space" body type', () => {
    renderContainer(sharedObjectWithBodyType('space'))
    expect(screen.getByTestId('space-container')).toBeDefined()
  })

  it('renders SpaceContainer for the "cdn.spacewave" body type', () => {
    renderContainer(sharedObjectWithBodyType('cdn.spacewave'))
    expect(screen.getByTestId('space-container')).toBeDefined()
  })

  it('renders the unknown-body-type fallback for other ids', () => {
    renderContainer(sharedObjectWithBodyType('counter'))
    expect(screen.queryByTestId('space-container')).toBeNull()
    expect(
      screen.getByText(/Unknown shared object body type: counter/),
    ).toBeDefined()
  })

  it('renders the unknown-body-type fallback when meta is missing', () => {
    renderContainer(sharedObjectWithBodyType(undefined))
    expect(screen.queryByTestId('space-container')).toBeNull()
    expect(screen.getByText(/Unknown shared object body type:/)).toBeDefined()
  })
})
