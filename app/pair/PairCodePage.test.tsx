import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ code: 'direct' })))
const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseRootResource = vi.hoisted(() => vi.fn(() => null))
const mockUseResourceValue = vi.hoisted(() => vi.fn(() => null))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@s4wave/app/quickstart/create.js', () => ({
  createLocalSession: vi.fn(),
}))

import { PairCodePage } from './PairCodePage.js'

describe('PairCodePage', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('surfaces PAIRING_FAILED from the PairVerifyStep status watch', async () => {
    let watchCount = 0
    const session = {
      acceptLocalPairingOffer: vi.fn(async () => ({
        answerPayload: 'answer-payload',
      })),
      watchPairingStatus: vi.fn(async function* () {
        if (watchCount++ === 0) {
          yield {
            status: PairingStatus.PairingStatus_PEER_CONNECTED,
            remotePeerId: 'remote-peer',
          }
          return
        }
        yield {
          status: PairingStatus.PairingStatus_FAILED,
          errorMessage: 'unsupported cross-NAT topology',
        }
      }),
    }

    render(<PairCodePage session={session as never} />)
    fireEvent.change(screen.getByPlaceholderText('Paste offer payload here…'), {
      target: { value: 'offer-payload' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Accept offer' }))

    expect(await screen.findByText('Verification failed')).toBeDefined()
    expect(screen.getByText('unsupported cross-NAT topology')).toBeDefined()
    expect(screen.queryByText('Waiting for other device')).toBeNull()
  })
})
