import { describe, expect, it } from 'vitest'

import {
  PAIRING_CODE_NOT_FOUND,
  PAIRING_FAILED,
  PAIRING_SERVICE_UNREACHABLE,
  pairingCodeInstructions,
  pairingErrorMessage,
} from './pairing-copy.js'

describe('pairingCodeInstructions', () => {
  it('does not tell a desktop user to open the desktop app', () => {
    const { heading, hint } = pairingCodeInstructions(true)
    expect(heading).toBe('Enter this code on your other device')
    expect(hint).not.toMatch(/open the .*desktop app/i)
    expect(hint).toMatch(/other device/i)
  })

  it('points a web user at the desktop app', () => {
    const { heading, hint } = pairingCodeInstructions(false)
    expect(heading).toMatch(/desktop app/i)
    expect(hint).toMatch(/open the spacewave desktop app/i)
  })
})

describe('pairingErrorMessage', () => {
  it('collapses relay 5xx and fetch failures to the reachability message', () => {
    expect(
      pairingErrorMessage('pairing relay returned 500: Failed to fetch'),
    ).toBe(PAIRING_SERVICE_UNREACHABLE)
    expect(pairingErrorMessage('Failed to fetch')).toBe(
      PAIRING_SERVICE_UNREACHABLE,
    )
    expect(
      pairingErrorMessage('NetworkError when attempting to fetch resource'),
    ).toBe(PAIRING_SERVICE_UNREACHABLE)
  })

  it('maps a missing message to the reachability message', () => {
    expect(pairingErrorMessage(null)).toBe(PAIRING_SERVICE_UNREACHABLE)
    expect(pairingErrorMessage('')).toBe(PAIRING_SERVICE_UNREACHABLE)
  })

  it('maps relay responses without exposing their details', () => {
    expect(
      pairingErrorMessage(
        'pairing relay returned 404: {"code":"not_found","message":"Pairing code not found or expired"}',
      ),
    ).toBe(PAIRING_CODE_NOT_FOUND)
    expect(
      pairingErrorMessage(
        'get pairing code: 404 not_found: Pairing code not found or expired',
      ),
    ).toBe(PAIRING_CODE_NOT_FOUND)
    expect(
      pairingErrorMessage(
        'get pairing code: 418 teapot: Unexpected relay response',
      ),
    ).toBe(PAIRING_FAILED)
    expect(
      pairingErrorMessage(
        'pairing relay returned 418: {"code":"teapot","message":"Unexpected relay response"}',
      ),
    ).toBe(PAIRING_FAILED)
    expect(
      pairingErrorMessage('pairing code conflict, retry with new code'),
    ).toBe('pairing code conflict, retry with new code')
    expect(pairingErrorMessage('unexpected pairing failure')).toBe(
      PAIRING_FAILED,
    )
  })
})
