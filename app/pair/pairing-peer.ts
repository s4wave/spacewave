// Pairing deep links resolve their code before the app boots. The relay
// deletes a code on first resolution and expires it ten minutes after the other
// device generated it, while a cold boot can take minutes, so the static page
// resolves the code at once and the pairing page hands the peer to the session.

const PAIR_ROUTE_PATTERN = /^#?\/pair\/([A-Za-z0-9]{8})$/
const PAIRING_PEER_PREFIX = 'spacewave-pairing-peer:'

/**
 * pairingRouteCode returns the uppercase pairing code of a /pair/CODE route,
 * with or without the leading hash, or undefined for any other route.
 */
export function pairingRouteCode(route: string): string | undefined {
  return PAIR_ROUTE_PATTERN.exec(route)?.[1].toUpperCase()
}

/**
 * resolvePairingPeer consumes a pairing code at the relay on the page origin,
 * where the browser release serves it, and remembers the registering peer for
 * this tab. Any failure leaves the code for the session to resolve.
 */
export async function resolvePairingPeer(code: string): Promise<void> {
  // Consume the code at the relay.
  let response: Response
  try {
    response = await fetch(`/api/pair/${code}`)
  } catch {
    return
  }
  const contentType = response.headers.get('Content-Type')
  if (!response.ok || contentType !== 'application/octet-stream') {
    return
  }

  // Remember the registering peer for the pairing page.
  const { PairingResponse } =
    await import('@s4wave/core/provider/spacewave/api/api.pb.js')
  const { peerId } = PairingResponse.fromBinary(
    new Uint8Array(await response.arrayBuffer()),
  )
  if (peerId) {
    sessionStorage.setItem(PAIRING_PEER_PREFIX + code, peerId)
  }
}

/**
 * resolvedPairingPeer returns the peer resolvePairingPeer stored for a code in
 * this tab, or undefined when the session must resolve the code itself.
 */
export function resolvedPairingPeer(code: string): string | undefined {
  return sessionStorage.getItem(PAIRING_PEER_PREFIX + code) ?? undefined
}
