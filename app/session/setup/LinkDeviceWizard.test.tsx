import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())
const mockUsePromise = vi.hoisted(() => vi.fn())
const mockUseResourceValue = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: vi.fn(() => mockNavigate),
  useParentPaths: vi.fn(() => ['/setup']),
  usePath: vi.fn(() => '/setup/link-device'),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: vi.fn(() => ({ value: null })) },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: mockUsePromise,
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/app/session/setup/LocalSessionOnboardingContext.js', () => ({
  useOptionalLocalSessionOnboardingContext: vi.fn(() => ({
    markProviderChoiceComplete: vi.fn(),
  })),
}))

vi.mock('./SetupPageLayout.js', () => ({
  SetupPageLayout: ({
    children,
    title,
    showHeader = true,
  }: {
    children: React.ReactNode
    title: string
    showHeader?: boolean
  }) => (
    <div>
      {showHeader && <h1>{title}</h1>}
      {children}
    </div>
  ),
}))

import { LinkDeviceWizard } from './LinkDeviceWizard.js'

describe('LinkDeviceWizard', () => {
  beforeEach(() => {
    mockNavigate.mockClear()
    mockUseSessionInfo.mockReset()
    mockUsePromise.mockReset()
    mockUseResourceValue.mockReset()
    mockUseSessionInfo.mockReturnValue({
      error: null,
      loading: false,
      providerId: 'local',
    })
    mockUsePromise.mockReturnValue({
      data: null,
      loading: false,
      error: null,
    })
    mockUseResourceValue.mockReturnValue(null)
  })

  afterEach(() => {
    cleanup()
  })

  it('renders cloud relay pairing options for spacewave providers', () => {
    mockUseSessionInfo.mockReturnValue({
      error: null,
      loading: false,
      providerId: 'spacewave',
    })

    render(<LinkDeviceWizard />)

    expect(screen.getByText('Generate code for another device')).toBeDefined()
    expect(screen.getByText('Enter a code from another device')).toBeDefined()
    expect(screen.queryByText('Show QR code')).toBeNull()
    expect(screen.queryByText('Scan QR code')).toBeNull()
  })

  it('groups options by which device shows the code and renames the direct QR paths', () => {
    render(<LinkDeviceWizard />)

    expect(screen.getByText('On this device')).toBeDefined()
    expect(screen.getByText('On your other device')).toBeDefined()
    expect(screen.getByText('Show QR code')).toBeDefined()
    expect(screen.getByText('Scan QR code')).toBeDefined()
    expect(screen.queryByText('Direct connection (show QR)')).toBeNull()
    expect(screen.queryByText('Direct connection (scan QR)')).toBeNull()
  })

  it('marks the generate-code path as the recommended default', () => {
    render(<LinkDeviceWizard />)

    const recommended = screen.getByText('Recommended')
    const option = recommended.closest('button')
    expect(option).not.toBeNull()
    expect(option?.textContent).toContain('Generate code for another device')
  })
  it('gives the pairing-code entry screen a descriptive heading', () => {
    render(<LinkDeviceWizard />)

    fireEvent.click(screen.getByText('Enter a code from another device'))

    expect(
      screen.getByRole('heading', { name: 'Pair another device' }),
    ).toBeDefined()
  })

  it('starts watching pairing status after the generated code resolves', async () => {
    const watchPairingStatus = vi.fn(async function* () {})
    const session = {
      generatePairingCode: vi.fn(),
      watchPairingStatus,
    }
    let code: string | null = null
    mockUseResourceValue.mockReturnValue(session)
    mockUsePromise.mockImplementation(() => ({
      data: code,
      loading: false,
      error: null,
    }))

    const { rerender } = render(<LinkDeviceWizard />)
    fireEvent.click(screen.getByText('Generate code for another device'))
    expect(watchPairingStatus).not.toHaveBeenCalled()

    code = 'ABCD1234'
    rerender(<LinkDeviceWizard />)

    await waitFor(() => {
      expect(watchPairingStatus).toHaveBeenCalledTimes(1)
    })
    expect(screen.queryByText('Continue')).toBeNull()
  })

  it('shows runtime-aware code copy without telling a desktop user to open the desktop app', async () => {
    const watchPairingStatus = vi.fn(async function* () {})
    const session = {
      generatePairingCode: vi.fn(),
      watchPairingStatus,
    }
    let code: string | null = null
    mockUseResourceValue.mockReturnValue(session)
    mockUsePromise.mockImplementation(() => ({
      data: code,
      loading: false,
      error: null,
    }))

    const { rerender } = render(<LinkDeviceWizard />)
    fireEvent.click(screen.getByText('Generate code for another device'))

    code = 'ABCD1234'
    rerender(<LinkDeviceWizard />)

    await waitFor(() => {
      expect(
        screen.getByText('Enter this code on your other device'),
      ).toBeDefined()
    })
    expect(screen.queryByText(/open the .*desktop app/i)).toBeNull()
    // The code renders as a grouped copyable chip.
    const codeButton = screen.getByRole('button', {
      name: 'Copy pairing code',
    })
    expect(codeButton.textContent).toContain('ABCD 1234')
    expect(screen.queryByTitle('Copy to clipboard')).toBeNull()
    expect(
      screen.getByRole('heading', { level: 2, name: 'Link My Device' }),
    ).toBeDefined()
    expect(
      screen.queryByRole('heading', { level: 1, name: 'Link My Device' }),
    ).toBeNull()
    let card: HTMLElement | null = codeButton
    while (
      card &&
      !card.className.toString().includes('border-foreground/20')
    ) {
      card = card.parentElement
    }
    expect(card).not.toBeNull()
    const generateButton = screen.getByRole('button', {
      name: 'Generate new code',
    })
    const waitingMessage = screen.getByText('Waiting for connection…')
    expect(
      generateButton.compareDocumentPosition(waitingMessage) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0)
  })

  it('surfaces PAIRING_FAILED from the VerifyStep status watch', async () => {
    const session = {
      createLocalPairingOffer: vi.fn(async () => ({
        offerPayload: 'offer-payload',
      })),
      acceptLocalPairingAnswer: vi.fn(async () => ({
        remotePeerId: 'remote-peer',
      })),
      watchPairingStatus: vi.fn(async function* () {
        yield {
          status: PairingStatus.PairingStatus_FAILED,
          errorMessage: 'unsupported cross-NAT topology',
        }
      }),
    }
    mockUseResourceValue.mockReturnValue(session)

    render(<LinkDeviceWizard />)
    fireEvent.click(screen.getByText('Show QR code'))
    const answerInput = await screen.findByPlaceholderText(
      'Paste answer payload here…',
    )
    fireEvent.change(answerInput, { target: { value: 'answer-payload' } })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    expect(await screen.findByText('Verification failed')).toBeDefined()
    expect(screen.getByText('unsupported cross-NAT topology')).toBeDefined()
    expect(screen.queryByText('Waiting for other device')).toBeNull()
  })
})
