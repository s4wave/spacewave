import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ code: 'direct' })))
const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseRootResource = vi.hoisted(() => vi.fn(() => null))
const mockUseResourceValue = vi.hoisted(() => vi.fn(() => null))
const mockPreparePairingSession = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@aptre/bldr-sdk/hooks/useResource.js')
  >()),
  useResourceValue: mockUseResourceValue,
}))

vi.mock('./prepare-session.js', () => ({
  preparePairingSession: mockPreparePairingSession,
}))

import { asyncValues } from '@s4wave/web/test/async-values.js'

import { PairCodePage } from './PairCodePage.js'

describe('PairCodePage', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    mockUseParams.mockReturnValue({ code: 'direct' })
    mockUseResourceValue.mockReturnValue(null)
  })

  it('opens the enrolled account returned by Home pairing', async () => {
    mockUseParams.mockReturnValue({ code: '' })
    mockUseResourceValue.mockReturnValue({} as never)
    const session = {
      completePairing: vi.fn(async () => 'source-peer'),
      watchPairingStatus: vi.fn(() =>
        asyncValues({ status: PairingStatus.PairingStatus_BOTH_CONFIRMED }),
      ),
      confirmPairing: vi.fn(async () => ({
        sessionListEntry: { sessionIndex: 7 },
      })),
      [Symbol.dispose]: vi.fn(),
    }
    mockPreparePairingSession.mockResolvedValue(session)

    render(<PairCodePage />)
    fireEvent.change(screen.getByPlaceholderText('XXXX XXXX'), {
      target: { value: 'AAAA AAAA' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Open account' }))

    expect(mockPreparePairingSession).toHaveBeenCalledOnce()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7' })
  })

  it('hands the peer resolved before boot to the session', async () => {
    mockUseParams.mockReturnValue({ code: 'AAAAAAAA' })
    mockUseResourceValue.mockReturnValue({} as never)
    sessionStorage.setItem('spacewave-pairing-peer:AAAAAAAA', 'source-peer')
    const session = {
      completePairing: vi.fn(() => new Promise<string>(() => {})),
      [Symbol.dispose]: vi.fn(),
    }
    mockPreparePairingSession.mockResolvedValue(session)

    render(<PairCodePage />)
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    await vi.waitFor(() =>
      expect(session.completePairing).toHaveBeenCalledWith(
        {
          code: 'AAAAAAAA',
          offerCurrentAccount: false,
          remotePeerId: 'source-peer',
        },
        expect.any(AbortSignal),
      ),
    )
    sessionStorage.clear()
  })

  it('shows the missing or expired message for a rejected pairing code', async () => {
    mockUseParams.mockReturnValue({ code: '' })
    const session = {
      completePairing: vi.fn(() =>
        Promise.reject(
          new Error(
            'pairing relay returned 404: {"code":"not_found","message":"Pairing code not found or expired"}',
          ),
        ),
      ),
    }

    render(<PairCodePage session={session as never} />)
    fireEvent.change(screen.getByPlaceholderText('XXXX XXXX'), {
      target: { value: 'AAAA AAAA' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    expect(
      await screen.findByText(
        'That pairing code was not found or has expired. Check the code and try again.',
      ),
    ).toBeDefined()
    expect(screen.queryByText(/pairing relay returned/)).toBeNull()
    expect(screen.getByDisplayValue('AAAA AAAA')).toBeDefined()
  })

  it('shows the fallback for an unmapped relay failure', async () => {
    mockUseParams.mockReturnValue({ code: '' })
    const session = {
      completePairing: vi.fn(() =>
        Promise.reject(
          new Error(
            'pairing relay returned 418: {"code":"teapot","message":"Unexpected relay response"}',
          ),
        ),
      ),
    }

    render(<PairCodePage session={session as never} />)
    fireEvent.change(screen.getByPlaceholderText('XXXX XXXX'), {
      target: { value: 'AAAA AAAA' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    expect(
      await screen.findByText(
        'Could not complete pairing. Check the code and try again.',
      ),
    ).toBeDefined()
    expect(screen.queryByText(/pairing relay returned/)).toBeNull()
  })

  it('surfaces PAIRING_FAILED from the PairVerifyStep status watch', async () => {
    let watchCount = 0
    const session = {
      acceptLocalPairingOffer: vi.fn(() =>
        Promise.resolve({ answerPayload: 'answer-payload' }),
      ),
      watchPairingStatus: vi.fn(() =>
        asyncValues(
          watchCount++ === 0
            ? {
                status: PairingStatus.PairingStatus_PEER_CONNECTED,
                remotePeerId: 'remote-peer',
              }
            : {
                status: PairingStatus.PairingStatus_FAILED,
                errorMessage: 'unsupported cross-NAT topology',
              },
        ),
      ),
    }

    render(<PairCodePage session={session as never} />)
    fireEvent.change(screen.getByPlaceholderText('Paste offer payload here…'), {
      target: { value: 'offer-payload' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Accept offer' }))

    expect(await screen.findByText('Pairing failed')).toBeDefined()
    expect(screen.getByText('unsupported cross-NAT topology')).toBeDefined()
    expect(screen.queryByText('Waiting for other device')).toBeNull()
  })
})
