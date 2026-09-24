import { afterEach, describe, expect, it, vi } from 'vitest'

import { PairingResponse } from '@s4wave/core/provider/spacewave/api/api.pb.js'

import {
  pairingRouteCode,
  resolvePairingPeer,
  resolvedPairingPeer,
} from './pairing-peer.js'

describe('pairing-peer', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    sessionStorage.clear()
  })

  it('reads the code from hash and pathname pairing routes', () => {
    expect(pairingRouteCode('#/pair/abcd1234')).toBe('ABCD1234')
    expect(pairingRouteCode('/pair/ABCD1234')).toBe('ABCD1234')
    expect(pairingRouteCode('#/pair/direct')).toBeUndefined()
    expect(pairingRouteCode('/pair/ABCD1234?x=1')).toBeUndefined()
  })

  it('remembers the peer the relay returns for a code', async () => {
    const fetch = vi.fn(
      async () =>
        new Response(
          PairingResponse.toBinary({ peerId: '12D3KooWSource' }).slice(),
          { headers: { 'Content-Type': 'application/octet-stream' } },
        ),
    )
    vi.stubGlobal('fetch', fetch)

    await resolvePairingPeer('ABCD1234')

    expect(fetch).toHaveBeenCalledWith('/api/pair/ABCD1234')
    expect(resolvedPairingPeer('ABCD1234')).toBe('12D3KooWSource')
  })

  it('leaves the code to the session when the origin has no relay', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response('<html></html>', {
            headers: { 'Content-Type': 'text/html' },
          }),
      ),
    )

    await resolvePairingPeer('ABCD1234')

    expect(resolvedPairingPeer('ABCD1234')).toBeUndefined()
  })
})
