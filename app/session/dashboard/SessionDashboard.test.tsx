import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ObjectTypeRegistration } from '@s4wave/sdk/objecttype/registry/registry.pb.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseVisibleQuickstartOptions = vi.hoisted(() => vi.fn())
const mockSetOpenMenu = vi.hoisted(() => vi.fn())
const mockSetStateAtom = vi.hoisted(() => vi.fn())
const mockDocs = vi.hoisted(() => vi.fn())
const mockGetObjectTypeIconComponent = vi.hoisted(() => vi.fn())
const mockClipboard = {
  writeText: vi.fn().mockResolvedValue(undefined),
}

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/app/nav-links.js', () => ({
  useNavLinks: () => ({
    blog: vi.fn(),
    changelog: vi.fn(),
    community: vi.fn(),
    docs: mockDocs,
    download: vi.fn(),
    legal: vi.fn(),
    support: vi.fn(),
  }),
}))

vi.mock('@s4wave/app/quickstart/QuickstartCommands.js', () => ({
  QuickstartCommands: () => null,
}))

vi.mock('@s4wave/app/quickstart/useQuickstartOptions.js', () => ({
  useVisibleQuickstartOptions: mockUseVisibleQuickstartOptions,
}))

vi.mock('@s4wave/app/session/setup/LocalSessionOnboardingContext.js', () => ({
  useSessionOnboardingState: () => ({
    onboarding: {
      backupComplete: true,
      lockComplete: true,
    },
    markBackupComplete: vi.fn(),
    markLockComplete: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => mockSetOpenMenu,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['session-settings'],
  useStateAtom: () => ['/', mockSetStateAtom],
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/web/ui/loading/LoadingInline.js', () => ({
  LoadingInline: ({ label }: { label: string }) => <span>{label}</span>,
}))
vi.mock('react-icons/lu', async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>()
  return {
    ...actual,
    LuLayers: ({ className }: { className?: string }) => (
      <svg className={className} role="img" aria-label="generic Space glyph" />
    ),
  }
})

vi.mock('@s4wave/web/space/object-tree.js', async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>()
  return {
    ...actual,
    getObjectTypeIconComponent: mockGetObjectTypeIconComponent,
  }
})

import { buildObjectTypeMetadataMap } from '@s4wave/web/space/object-tree.js'
import type { ObjectTypeMetadataById } from '@s4wave/web/space/object-tree.js'

import { SessionDashboard } from './SessionDashboard.js'

function QuickstartIcon({ className }: { className?: string }) {
  return <svg className={className} aria-hidden="true" />
}

function expectTextBefore(first: string, second: string) {
  const text = document.body.textContent ?? ''
  expect(text.indexOf(first)).toBeGreaterThanOrEqual(0)
  expect(text.indexOf(second)).toBeGreaterThanOrEqual(0)
  expect(text.indexOf(first)).toBeLessThan(text.indexOf(second))
}

describe('SessionDashboard', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockSetOpenMenu.mockReset()
    mockSetStateAtom.mockReset()
    mockDocs.mockReset()
    mockUseVisibleQuickstartOptions.mockReset()
    mockGetObjectTypeIconComponent.mockReset()
    Object.defineProperty(navigator, 'clipboard', {
      value: mockClipboard,
      writable: true,
      configurable: true,
    })
    mockClipboard.writeText.mockClear()
    mockUseVisibleQuickstartOptions.mockReturnValue([
      {
        id: 'account',
        name: 'Sign in or create account',
        description: 'Access your account',
        category: 'account',
        icon: QuickstartIcon,
        path: '/login',
      },
      {
        id: 'pair',
        name: 'Enter a device pairing code',
        description: 'Link to an existing device via pairing code',
        category: 'account',
        icon: QuickstartIcon,
        path: '/pair',
      },
      {
        id: 'space',
        name: 'Create an Empty Space',
        description: 'Start with a blank space',
        category: 'storage',
        icon: QuickstartIcon,
      },
      {
        id: 'drive',
        name: 'Create a Drive',
        description: 'Drive workspace',
        category: 'storage',
        icon: QuickstartIcon,
      },
      {
        id: 'canvas',
        name: 'Create a Canvas',
        description: 'Visual workspace',
        category: 'storage',
        icon: QuickstartIcon,
      },
    ])
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('places join actions above empty-dashboard creation actions', () => {
    render(<SessionDashboard spaces={[]} onQuickstartClick={vi.fn()} />)

    expect(screen.getByText('Continue')).toBeDefined()
    expect(screen.getByText('Other starts')).toBeDefined()
    expect(screen.getByText('Browse templates')).toBeDefined()
    expectTextBefore('Join Space', 'Link a device')
    expectTextBefore('Link a device', 'Create a Drive')
    expectTextBefore('Create a Drive', 'Create a Canvas')
    const linkDeviceItem = screen
      .getByText('Link a device')
      .closest('[cmdk-item]')
    expect(linkDeviceItem?.getAttribute('class')).toContain(
      'data-[selected=true]:!bg-transparent',
    )
    expect(linkDeviceItem?.getAttribute('class')).toContain(
      'hover:!bg-background-card/30',
    )
  })

  it('keeps returning sessions join-first before spaces and create actions', () => {
    render(
      <SessionDashboard
        spaces={[{ id: 'space-1', name: 'Alpha Space' }]}
        onQuickstartClick={vi.fn()}
      />,
    )

    expect(screen.queryByText('Continue')).toBeNull()
    expectTextBefore('Join Space', 'Link a device')
    expectTextBefore('Link a device', 'Alpha Space')
    expectTextBefore('Alpha Space', 'Create a Drive')
  })

  it('copies the full space ID from each space affordance', () => {
    const ownedSpaceId = 'space-owned-01HZY4PZQWVN9A1K4J4Y7P6C3R'
    const sharedSpaceId = 'space-shared-01HZY4Q7P4CQVWDTVR9N6DJ6ZY'

    render(
      <SessionDashboard
        spaces={[
          {
            id: ownedSpaceId,
            name: 'Alpha Space',
          },
          {
            id: sharedSpaceId,
            name: 'Beta Space',
          },
        ]}
        onQuickstartClick={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Copy Alpha Space ID' }))
    expect(mockClipboard.writeText).toHaveBeenCalledWith(ownedSpaceId)

    fireEvent.click(screen.getByRole('button', { name: 'Copy Beta Space ID' }))
    expect(mockClipboard.writeText).toHaveBeenCalledWith(sharedSpaceId)
  })

  it('shows an overflow cue and the owner-provided source for every space row', () => {
    const spaces = [
      {
        id: 'space-owned-drive',
        name: 'Drive',
        source: 'created',
        expectedSource: 'Owned Space',
      },
      {
        id: 'space-shared-kv',
        name: 'KV Store',
        source: 'shared',
        expectedSource: 'Shared Space',
      },
      {
        id: 'space-owned-sql',
        name: 'SQL Database',
        source: 'created',
        expectedSource: 'Owned Space',
      },
    ]

    render(
      <SessionDashboard
        spaces={spaces.map(({ expectedSource: _, ...space }) => space)}
        onQuickstartClick={vi.fn()}
      />,
    )

    expect(screen.getByRole('note').textContent).toContain(
      'Scroll to see all 3 spaces',
    )
    for (const { name, expectedSource } of spaces) {
      const row = screen.getByText(name).closest('[cmdk-item]')
      expect(row).not.toBeNull()
      expect(row?.textContent).toContain(expectedSource)
    }
  })

  it('renders registered ObjectType glyphs and the generic Space fallback per row', () => {
    const objectTypes: ObjectTypeRegistration[] = [
      {
        typeId: 'canvas',
        registrationId: 1,
        metadata: { iconName: 'paintbrush' },
      },
      {
        typeId: 'git/repo',
        registrationId: 2,
        metadata: { iconName: 'git-branch' },
      },
    ]
    const objectTypeMetadataById = buildObjectTypeMetadataMap(objectTypes)
    mockGetObjectTypeIconComponent.mockImplementation(
      (typeId: string, metadataById?: ObjectTypeMetadataById) => {
        const glyphName = metadataById?.get(typeId)?.iconName ?? typeId
        return function RegisteredObjectTypeGlyph({
          className,
        }: {
          className?: string
        }) {
          return (
            <svg
              className={className}
              role="img"
              aria-label={`${glyphName} glyph`}
            />
          )
        }
      },
    )

    render(
      <SessionDashboard
        spaces={[
          {
            id: 'space-canvas',
            name: 'Canvas Space',
            source: 'created',
            objectType: 'canvas',
          },
          {
            id: 'space-repo',
            name: 'Repository Space',
            source: 'shared',
            objectType: 'git/repo',
          },
          {
            id: 'space-generic',
            name: 'Generic Space',
            source: 'created',
          },
        ]}
        objectTypeMetadataById={objectTypeMetadataById}
        onQuickstartClick={vi.fn()}
      />,
    )

    const canvasRow = screen
      .getByText('Canvas Space')
      .closest<HTMLElement>('[cmdk-item]')
    const repositoryRow = screen
      .getByText('Repository Space')
      .closest<HTMLElement>('[cmdk-item]')
    const genericRow = screen
      .getByText('Generic Space')
      .closest<HTMLElement>('[cmdk-item]')
    expect(canvasRow).not.toBeNull()
    expect(repositoryRow).not.toBeNull()
    expect(genericRow).not.toBeNull()
    expect(
      within(canvasRow!).getByRole('img', { name: 'paintbrush glyph' }),
    ).toBeDefined()
    expect(
      within(repositoryRow!).getByRole('img', { name: 'git-branch glyph' }),
    ).toBeDefined()
    expect(
      within(genericRow!).getByRole('img', { name: 'generic Space glyph' }),
    ).toBeDefined()
  })

  it('keeps join actions available for read-only sessions without creation actions', () => {
    render(
      <SessionDashboard spaces={[]} readOnly onQuickstartClick={vi.fn()} />,
    )

    expect(screen.queryByText('Continue')).toBeNull()
    expect(screen.queryByText('Create a Drive')).toBeNull()
    expect(screen.getByText('Join Space')).toBeDefined()
    expectTextBefore('Join Space', 'Link a device')
  })

  it('opens Docs through the shared navigation owner', () => {
    render(<SessionDashboard spaces={[]} onQuickstartClick={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Docs' }))

    expect(mockDocs).toHaveBeenCalledOnce()
  })
})
