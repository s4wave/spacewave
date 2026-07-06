import { describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { MountSharedObjectResponse } from '@s4wave/sdk/session/session.pb.js'
import { SharedObject } from '@s4wave/sdk/sobject/sobject.js'

import { SharedObjectDetails } from './SharedObjectDetails.js'

const mockMeta: MountSharedObjectResponse = {
  sharedObjectId: 'test-object-id',
  blockStoreId: 'test-blockstore-id',
  peerId: 'test-peer-id',
  sharedObjectMeta: {
    bodyType: 'space',
  },
}

const mockSharedObject = {
  meta: mockMeta,
  resourceRef: {
    resourceId: 1,
    released: false,
  },
  id: 1,
  client: {},
  service: {},
  mountSharedObjectBody: vi.fn(),
} as unknown as SharedObject

// SpaceDetailsSurface fills the viewport and applies the bg-background-primary
// token so the screenshot captures the Space details panel over its real
// backdrop. The panel renders fully: real CollapsibleSection, InfoCard,
// CopyableField, and SpaceMembersPanel off the provided context resources.
function SpaceDetailsSurface() {
  return (
    <div
      data-testid="space-details-surface"
      className="bg-background-primary text-foreground fixed inset-0"
    >
      <SpaceContainerContext.Provider
        spaceId="test-space"
        spaceState={{ ready: true }}
        spaceSharingState={{}}
        spaceWorldResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        spaceWorld={{} as never}
        navigateToRoot={vi.fn()}
        navigateToObjects={vi.fn()}
        buildObjectUrls={vi.fn()}
        navigateToSubPath={vi.fn()}
      >
        <SharedObjectContext.Provider
          resource={{
            value: mockSharedObject,
            loading: false,
            error: null,
            retry: vi.fn(),
          }}
        >
          <SharedObjectDetails
            displayName="Team Space"
            canRename={true}
            onRenameStart={vi.fn()}
            onCloseClick={vi.fn()}
            onExportClick={vi.fn()}
            onDeleteClick={vi.fn()}
          />
        </SharedObjectContext.Provider>
      </SpaceContainerContext.Provider>
    </div>
  )
}

function surfaceBackground(): string {
  const el = document.querySelector('[data-testid="space-details-surface"]')
  if (!(el instanceof HTMLElement)) {
    throw new Error('space details surface was not rendered')
  }
  return getComputedStyle(el).backgroundColor
}

async function capture(name: string) {
  return page.screenshot({
    path: `__screenshots__/space-details/${name}.png`,
  })
}

describe('space details panel browser render', () => {
  it('renders the header, section list, and sharing state on a desktop viewport', async () => {
    await render(<SpaceDetailsSurface />)

    await expect.element(page.getByText('Team Space')).toBeInTheDocument()
    await expect
      .element(page.getByText('Sharing', { exact: true }))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Identifiers', { exact: true }))
      .toBeInTheDocument()
    // Sharing is the default-open section, so its empty state renders inline.
    await expect
      .element(page.getByText('No users added yet'))
      .toBeInTheDocument()

    // The bg-background-primary token must resolve to a real color, proving the
    // app stylesheet loaded rather than falling back to a transparent root.
    expect(surfaceBackground()).not.toBe('rgba(0, 0, 0, 0)')
    expect(surfaceBackground()).not.toBe('transparent')

    await capture('panel-desktop')
    await cleanup()
  })

  it('expands the identifiers section to reveal copyable object metadata', async () => {
    await render(<SpaceDetailsSurface />)

    await page.getByText('Identifiers', { exact: true }).click()

    await expect.element(page.getByText('Object ID')).toBeInTheDocument()
    await expect.element(page.getByText('test-object-id')).toBeInTheDocument()
    await expect.element(page.getByText('test-peer-id')).toBeInTheDocument()

    await capture('identifiers-expanded')
    await cleanup()
  })

  it('keeps the space details panel within a narrow viewport without horizontal overflow', async () => {
    await page.viewport(390, 844)

    await render(<SpaceDetailsSurface />)

    await expect.element(page.getByText('Team Space')).toBeInTheDocument()
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      window.innerWidth,
    )

    await capture('panel-narrow')
    await cleanup()
  })
})
