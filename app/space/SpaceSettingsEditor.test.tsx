import React from 'react'
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceSettingsEditor } from './SpaceSettingsEditor.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import type { SpaceState } from '@s4wave/sdk/space/space.pb.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'

vi.mock('@s4wave/web/ui/ObjectKeySelector.js', () => ({
  ObjectKeySelector: ({
    value,
    onChange,
    placeholder,
    disabled,
  }: {
    value: string
    onChange: (v: string) => void
    placeholder?: string
    disabled?: boolean
  }) => (
    <button
      data-testid="object-key-selector"
      data-value={value}
      data-placeholder={placeholder}
      disabled={disabled}
      onClick={() => onChange('new/path')}
    >
      {value || placeholder}
    </button>
  ),
}))

describe('SpaceSettingsEditor', () => {
  const mockSpaceWorld = {
    applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
    deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
    getEngine: vi.fn(),
  } as unknown as EngineWorldState

  const mockWorldResource: Resource<EngineWorldState> = {
    value: mockSpaceWorld,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
  const mockRenameStart = vi.fn()

  const mockSpaceState: SpaceState = {
    ready: true,
    worldContents: {
      objects: [
        { objectKey: 'object-layout/main', objectType: 'alpha/object-layout' },
        { objectKey: 'files', objectType: 'unixfs/fs-node' },
      ],
    },
    settings: { indexPath: 'object-layout/main', pluginIds: ['spacewave-app'] },
  }

  function renderEditor(
    canEdit: boolean,
    spaceState: SpaceState = mockSpaceState,
    props?: Partial<React.ComponentProps<typeof SpaceSettingsEditor>>,
  ) {
    return render(
      <SpaceContainerContext.Provider
        spaceId="test-space"
        spaceState={spaceState}
        spaceWorldResource={mockWorldResource}
        spaceWorld={mockSpaceWorld}
        navigateToRoot={vi.fn()}
        navigateToObjects={vi.fn()}
        buildObjectUrls={vi.fn()}
        navigateToSubPath={vi.fn()}
      >
        <SpaceSettingsEditor
          canEdit={canEdit}
          canRename={true}
          displayName="Test Space"
          onRenameStart={mockRenameStart}
          {...props}
        />
      </SpaceContainerContext.Provider>,
    )
  }

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders "Settings" heading', () => {
    renderEditor(true)
    expect(screen.getByText('Settings')).toBeDefined()
  })

  it('omits the outer heading when embedded', () => {
    renderEditor(true, mockSpaceState, { embedded: true })
    expect(screen.queryByText('Settings')).toBeNull()
  })

  it('renders "Index Path" label', () => {
    renderEditor(true)
    expect(screen.getByText('Index Path')).toBeDefined()
  })

  it('shows ObjectKeySelector when canEdit is true', () => {
    renderEditor(true)
    expect(screen.getByTestId('object-key-selector')).toBeDefined()
  })

  it('shows current indexPath value in the selector', () => {
    renderEditor(true)
    const selector = screen.getByTestId('object-key-selector')
    expect(selector.getAttribute('data-value')).toBe('object-layout/main')
    expect(selector.textContent).toBe('object-layout/main')
  })

  it('shows plain text "Not set" when canEdit is false and no indexPath', () => {
    const noIndexState: SpaceState = {
      ready: true,
      worldContents: { objects: [] },
      settings: {},
    }
    renderEditor(false, noIndexState)
    expect(screen.getByText('Not set')).toBeDefined()
    expect(screen.queryByTestId('object-key-selector')).toBeNull()
  })

  it('shows plain text indexPath when canEdit is false', () => {
    renderEditor(false)
    expect(screen.getByText('object-layout/main')).toBeDefined()
    expect(screen.queryByTestId('object-key-selector')).toBeNull()
  })

  it('shows the display name and rename affordance', () => {
    renderEditor(true)
    expect(screen.getByText('Test Space').className).toContain('text-xs')
    expect(screen.getByText('Rename')).toBeDefined()
  })

  it('triggers onRenameStart when the display name is double-clicked', () => {
    renderEditor(true)
    fireEvent.doubleClick(screen.getByText('Test Space'))
    expect(mockRenameStart).toHaveBeenCalledTimes(1)
  })

  it('triggers onRenameStart when the rename button is clicked', async () => {
    const user = userEvent.setup()
    renderEditor(true)
    await user.click(screen.getByText('Rename'))
    expect(mockRenameStart).toHaveBeenCalledTimes(1)
  })

  it('renders plain display name when canRename is false', () => {
    renderEditor(true, mockSpaceState, { canRename: false })
    expect(screen.getByText('Test Space')).toBeDefined()
    expect(screen.queryByText('Rename')).toBeNull()
  })

  it('calls applyWorldOp with correct op ID when selector changes', async () => {
    const user = userEvent.setup()
    renderEditor(true)
    const selector = screen.getByTestId('object-key-selector')
    await user.click(selector)
    expect(mockSpaceWorld.applyWorldOp).toHaveBeenCalledWith(
      SET_SPACE_SETTINGS_OP_ID,
      expect.any(Uint8Array),
      '',
    )
    const opData = vi.mocked(mockSpaceWorld.applyWorldOp).mock.calls[0]?.[1]
    const op = SetSpaceSettingsOp.fromBinary(opData as Uint8Array)
    expect(op.settings?.indexPath).toBe('new/path')
    expect(op.settings?.pluginIds).toEqual(['spacewave-app'])
  })

  it('does not call applyWorldOp when new path matches current indexPath', async () => {
    const user = userEvent.setup()
    const matchingState: SpaceState = {
      ready: true,
      worldContents: { objects: [] },
      settings: { indexPath: 'new/path' },
    }
    renderEditor(true, matchingState)
    const selector = screen.getByTestId('object-key-selector')
    await user.click(selector)
    expect(mockSpaceWorld.applyWorldOp).not.toHaveBeenCalled()
  })
})
