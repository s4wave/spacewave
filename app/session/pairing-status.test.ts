import { describe, expect, it } from 'vitest'

import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'

import { pairingStatusReachedPeer } from './pairing-status.js'

describe('pairingStatusReachedPeer', () => {
  it.each([
    PairingStatus.PairingStatus_PEER_CONNECTED,
    PairingStatus.PairingStatus_VERIFYING_EMOJI,
    PairingStatus.PairingStatus_WAITING_FOR_REMOTE_CONFIRM,
    PairingStatus.PairingStatus_BOTH_CONFIRMED,
    PairingStatus.PairingStatus_VERIFIED,
  ])('accepts status %s as reached', (status) => {
    expect(pairingStatusReachedPeer(status)).toBe(true)
  })

  it.each([
    undefined,
    PairingStatus.PairingStatus_IDLE,
    PairingStatus.PairingStatus_WAITING_FOR_PEER,
    PairingStatus.PairingStatus_FAILED,
    PairingStatus.PairingStatus_CONNECTION_TIMEOUT,
  ])('rejects status %s before or outside the reached stage', (status) => {
    expect(pairingStatusReachedPeer(status)).toBe(false)
  })
})
